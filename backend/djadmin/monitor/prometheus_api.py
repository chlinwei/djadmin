from __future__ import annotations

import json
from typing import Any
from urllib import error as urllib_error
from urllib import parse as urllib_parse
from urllib import request as urllib_request

from sys_config.models import SysConfig


PROMETHEUS_BASE_URL_KEY = 'sys.monitor.prometheus.base_url'
DEFAULT_PROMETHEUS_BASE_URL = 'http://10.25.66.150:9999'


def get_prometheus_base_url() -> str:
    cfg, _ = SysConfig.objects.get_or_create(
        key=PROMETHEUS_BASE_URL_KEY,
        defaults={
            'value': DEFAULT_PROMETHEUS_BASE_URL,
            'default_value': DEFAULT_PROMETHEUS_BASE_URL,
            'value_type': 'string',
            'name': 'Prometheus 基础地址',
            'description': '监控中心请求 Prometheus HTTP API 的基础地址',
            'is_readonly': False,
        },
    )
    return str(cfg.value or DEFAULT_PROMETHEUS_BASE_URL).rstrip('/')


def _build_url(path: str, params: dict[str, Any] | None = None) -> str:
    base_url = get_prometheus_base_url()
    normalized_path = path if path.startswith('/') else f'/{path}'
    if not params:
        return f'{base_url}{normalized_path}'
    query = urllib_parse.urlencode(params)
    return f'{base_url}{normalized_path}?{query}'


def api_get(path: str, params: dict[str, Any] | None = None, timeout_seconds: int = 8) -> dict[str, Any]:
    url = _build_url(path, params)
    req = urllib_request.Request(url=url, method='GET')
    req.add_header('Accept', 'application/json')
    try:
        with urllib_request.urlopen(req, timeout=timeout_seconds) as resp:
            payload_text = resp.read().decode('utf-8', errors='replace')
    except urllib_error.URLError as exc:
        return {
            'ok': False,
            'status': 'error',
            'error': f'prometheus request failed: {exc}',
            'data': {},
        }

    try:
        payload = json.loads(payload_text)
    except json.JSONDecodeError:
        return {
            'ok': False,
            'status': 'error',
            'error': f'prometheus invalid json response: {payload_text[:500]}',
            'data': {},
        }

    is_success = str(payload.get('status') or '').lower() == 'success'
    return {
        'ok': is_success,
        'status': str(payload.get('status') or ''),
        'error': str(payload.get('error') or ''),
        'errorType': str(payload.get('errorType') or ''),
        'warnings': payload.get('warnings') or [],
        'data': payload.get('data') if isinstance(payload.get('data'), dict) else payload.get('data') or {},
    }


def query_instant(promql: str, timeout_seconds: int = 8) -> dict[str, Any]:
    return api_get('/api/v1/query', params={'query': promql}, timeout_seconds=timeout_seconds)
