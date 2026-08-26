"""OpenSearch REST 客户端封装。

用标准库 urllib，与 monitor/prometheus_api.py 保持一致，不额外引入 HTTP 依赖。
"""
import base64
import json
import ssl
from urllib import error as urllib_error
from urllib import parse as urllib_parse
from urllib import request as urllib_request

from assets.credential_crypto import decrypt_secret


class OpenSearchError(Exception):
    pass


class OpenSearchClient:
    def __init__(self, cluster):
        self.cluster = cluster
        self.hosts = cluster.host_list
        if not self.hosts:
            raise OpenSearchError('集群未配置连接地址')
        self.timeout = cluster.request_timeout or 10

    def _ssl_context(self):
        if self.cluster.verify_tls:
            if self.cluster.ca_cert:
                return ssl.create_default_context(cadata=self.cluster.ca_cert)
            return ssl.create_default_context()
        # 自签证书场景：按配置显式关闭校验。
        context = ssl.create_default_context()
        context.check_hostname = False
        context.verify_mode = ssl.CERT_NONE
        return context

    def _auth_header(self):
        username = str(self.cluster.username or '').strip()
        if not username:
            return None
        password = str(decrypt_secret(self.cluster.password) or '')
        token = base64.b64encode(f'{username}:{password}'.encode('utf-8')).decode('ascii')
        return f'Basic {token}'

    def _request(self, method, path, payload=None, params=None):
        body = json.dumps(payload).encode('utf-8') if payload is not None else None
        query = f'?{urllib_parse.urlencode(params)}' if params else ''
        auth = self._auth_header()
        context = self._ssl_context()

        last_error = None
        # 多地址时逐个尝试，任一可用即返回，避免单节点故障导致功能整体不可用。
        for host in self.hosts:
            url = f"{host.rstrip('/')}/{path.lstrip('/')}{query}"
            request = urllib_request.Request(url, data=body, method=method)
            request.add_header('Content-Type', 'application/json')
            if auth:
                request.add_header('Authorization', auth)
            try:
                with urllib_request.urlopen(request, timeout=self.timeout, context=context) as response:
                    raw = response.read()
            except urllib_error.HTTPError as exc:
                detail = exc.read().decode('utf-8', errors='replace')[:500]
                raise OpenSearchError(f'{exc.code}: {detail}') from exc
            except (urllib_error.URLError, ssl.SSLError, OSError) as exc:
                last_error = str(exc)
                continue
            if not raw:
                return {}
            try:
                return json.loads(raw.decode('utf-8'))
            except (ValueError, UnicodeDecodeError) as exc:
                raise OpenSearchError(f'响应不是合法 JSON: {exc}') from exc
        raise OpenSearchError(last_error or '所有连接地址均不可用')

    def ping(self):
        """返回集群基础信息，用于连接测试。"""
        info = self._request('GET', '/')
        health = self._request('GET', '/_cluster/health')
        version = info.get('version') or {}
        return {
            'cluster_name': info.get('cluster_name', ''),
            'distribution': version.get('distribution', ''),
            'version': version.get('number', ''),
            'status': health.get('status', ''),
            'number_of_nodes': health.get('number_of_nodes', 0),
        }

    def get_pipeline(self, name):
        return self._request('GET', f'/_ingest/pipeline/{name}')

    def put_pipeline(self, name, body):
        return self._request('PUT', f'/_ingest/pipeline/{name}', payload=body)

    def delete_pipeline(self, name):
        return self._request('DELETE', f'/_ingest/pipeline/{name}')

    def simulate_pipeline(self, name, docs):
        payload = {'docs': [{'_source': item} for item in docs]}
        return self._request('POST', f'/_ingest/pipeline/{name}/_simulate', payload=payload)

    def simulate_pipeline_body(self, body, docs):
        """用未保存的 pipeline 定义直接试跑，供解析规则调试使用。"""
        payload = {'pipeline': body, 'docs': [{'_source': item} for item in docs]}
        return self._request('POST', '/_ingest/pipeline/_simulate', payload=payload)

    def search(self, index, body):
        return self._request('POST', f'/{index}/_search', payload=body)
