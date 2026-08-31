import json

from django.contrib.auth.hashers import check_password
from django.http import JsonResponse
from django.utils.deprecation import MiddlewareMixin
from django.utils import timezone
from jwt import ExpiredSignatureError, InvalidTokenError, PyJWTError
from rest_framework_jwt.settings import api_settings

from djadmin.utils import Response_error_str
from user.utils import getCurrentUser


class JwtAuthenticationMiddleware(MiddlewareMixin):

    _AUDIT_SKIP_PREFIXES = ('/sys/audit/', '/media', '/static')
    _AUDIT_SKIP_PATHS = {'/sys/login'}
    _AGENT_PATH_PREFIXES = ('/api/agent/', '/sys/agent/')
    _AGENT_RUNTIME_PATHS = (
        '/api/agent/configs/',
    )
    _PROMETHEUS_HTTP_SD_PATHS = {
        '/monitor/prometheus/http-sd/',
        '/monitor/targets/prometheus/http-sd/',
    }
    # backend 替代 Alertmanager 接收 Prometheus notifier 主动推送的告警：调用方是 Prometheus
    # 而非登录用户，天然拿不到 JWT；鉴权与 dj-agent 共用全局 ApiToken（见 _authenticate_prometheus_request），
    # 这里先豁免 JWT 再转到 ApiToken 校验，否则会在到达视图前就被这个中间件拦成 301。
    _ALERT_WEBHOOK_PATHS = {
        '/monitor/alert-webhook/api/v2/alerts',
    }

    def process_request(self, request):
        request._operation_audit_started_at = timezone.now()
        request._operation_audit_request_data = self._extract_request_data(request)
        white_list = ["/sys/login"]  # 请求白名单
        path = request.path
        if path in self._PROMETHEUS_HTTP_SD_PATHS or path in self._ALERT_WEBHOOK_PATHS:
            return self._authenticate_prometheus_request(request)
        if self._is_agent_path(path):
            return self._authenticate_agent_or_user_request(request)

        if path not in white_list and not path.startswith("/media"):
            token = request.META.get('HTTP_AUTHORIZATION')
            try:
                jwt_decode_handler = api_settings.JWT_DECODE_HANDLER
                if not callable(jwt_decode_handler):
                    raise InvalidTokenError()
                jwt_decode_handler(token)
                
            except ExpiredSignatureError:
                return Response_error_str('Token过期，请重新登录！', code=301, data={})
            except InvalidTokenError:
                return Response_error_str('Token验证失败！', code=301, data={})
            except PyJWTError:
                return Response_error_str('Token验证异常！', code=301, data={})
        else:
            return None

    def _is_agent_path(self, path):
        if not path:
            return False
        return any(path.startswith(prefix) for prefix in self._AGENT_PATH_PREFIXES)

    def _is_agent_runtime_path(self, path):
        if not path:
            return False
        return any(path.startswith(prefix) for prefix in self._AGENT_RUNTIME_PATHS)

    def _decode_jwt_token(self, token):
        jwt_decode_handler = api_settings.JWT_DECODE_HANDLER
        if not callable(jwt_decode_handler):
            raise InvalidTokenError()
        jwt_decode_handler(token)

    def _authenticate_agent_or_user_request(self, request):
        path = request.path
        # agent 运行时链路（configs）只接受 ApiToken。
        if self._is_agent_runtime_path(path):
            return self._authenticate_agent_request(request)

        # 控制面接口（create/query/cancel/retry...）走用户 JWT，便于前端管理页面直接调用。
        token = request.META.get('HTTP_AUTHORIZATION')
        try:
            self._decode_jwt_token(token)
            return None
        except ExpiredSignatureError:
            return Response_error_str('Token过期，请重新登录！', code=301, data={})
        except InvalidTokenError:
            return Response_error_str('Token验证失败！', code=301, data={})
        except PyJWTError:
            return Response_error_str('Token验证异常！', code=301, data={})

    def _authenticate_agent_request(self, request):
        token = (request.META.get('HTTP_AUTHORIZATION') or '').strip()
        if token == '':
            return Response_error_str('Api Token不能为空', code=301, data={})

        record = self._validate_api_token(token)
        if record is None:
            return Response_error_str('Api Token验证失败！', code=301, data={})
        request.agent_id = record.agent_id
        request.bind_mode = record.bind_mode
        return None

    def _validate_api_token(self, token):
        """校验全局 ApiToken（agent 共享 token / api 绑定 token 同一套）。命中则刷新使用时间并返回记录，否则 None。"""
        if not token:
            return None

        from user.models import ApiToken

        now = timezone.now()
        # Token 哈希不可逆，必须遍历候选记录逐个校验哈希。
        for record in ApiToken.objects.filter(is_active=True):
            # 业务规则：agent 共享 token 永不过期，仅 api 绑定 token 受 expires_at 约束。
            if record.bind_mode == 'api' and record.expires_at is not None and record.expires_at <= now:
                continue
            if not check_password(token, record.token_hash):
                continue

            record.last_used_at = now
            record.save(update_fields=['last_used_at'])
            return record

        return None

    def _extract_prometheus_token(self, request):
        # http_sd 走 URL 上的 ?token=，alert-webhook 走 Authorization: Bearer <token>，统一取出明文 token。
        query_token = str(request.GET.get('token') or '').strip()
        if query_token:
            return query_token
        auth = (request.META.get('HTTP_AUTHORIZATION') or '').strip()
        if auth.lower().startswith('bearer '):
            return auth[len('Bearer '):].strip()
        return auth

    def _authenticate_prometheus_request(self, request):
        # Prometheus http_sd / alert-webhook 与 dj-agent 共用全局 ApiToken 认证，不再维护
        # 独立的 monitor.prometheus.*_token 参数。调用方是 Prometheus（非登录用户），校验失败
        # 返回原生 403，便于 Prometheus 侧按 HTTP 状态识别鉴权失败。
        token = self._extract_prometheus_token(request)
        if self._validate_api_token(token) is None:
            return JsonResponse({'error': 'forbidden'}, status=403)
        return None

    def process_response(self, request, response):
        self._write_operation_audit_log(request, response)
        return response

    def _should_skip_operation_audit(self, request, response):
        path = getattr(request, 'path', '') or ''
        if not path:
            return True
        if request.method == 'GET':
            return True
        if request.method == 'OPTIONS':
            return True
        if path in self._AUDIT_SKIP_PATHS:
            return True
        if any(path.startswith(prefix) for prefix in self._AUDIT_SKIP_PREFIXES):
            return True
        if getattr(request, 'resolver_match', None) is None:
            return True
        if getattr(response, 'status_code', None) == 404:
            return True
        return False

    def _write_operation_audit_log(self, request, response):
        if self._should_skip_operation_audit(request, response):
            return

        try:
            payload = getCurrentUser(request)
        except Exception:
            return

        user_id = payload.get('user_id')
        username = payload.get('username') or ''
        if not user_id and not username:
            return

        request_data = getattr(request, '_operation_audit_request_data', None)
        if request_data is None:
            request_data = self._extract_request_data(request)
        response_data = self._extract_response_data(response)

        started_at = getattr(request, '_operation_audit_started_at', None)
        duration_ms = None
        if started_at is not None:
            duration_ms = max(int((timezone.now() - started_at).total_seconds() * 1000), 0)

        message = self._extract_response_message(response)
        from audit.models import OperationAuditLog

        OperationAuditLog.objects.create(
            username=username,
            user_id=user_id,
            method=getattr(request, 'method', '') or '',
            path=getattr(request, 'path', '') or '',
            route_name=getattr(getattr(request, 'resolver_match', None), 'view_name', '') or '',
            client_ip=self._get_client_ip(request),
            user_agent=request.META.get('HTTP_USER_AGENT', '')[:255],
            request_data=request_data,
            response_data=response_data,
            status_code=getattr(response, 'status_code', 200) or 200,
            duration_ms=duration_ms,
            message=message,
        )

    @staticmethod
    def _truncate_text(value, limit=4000):
        if not value:
            return ''
        return value[:limit]

    def _extract_request_data(self, request):
        if getattr(request, 'method', '') == 'GET':
            return ''

        content_type = (request.META.get('CONTENT_TYPE') or '').lower()
        body_payload = None
        try:
            if 'application/json' in content_type:
                raw_body = request.body.decode('utf-8') if request.body else ''
                if raw_body:
                    body_payload = json.loads(raw_body)
            elif 'multipart/form-data' in content_type:
                payload = request.POST.dict()
                if payload:
                    body_payload = payload
            else:
                payload = request.POST.dict()
                if payload:
                    body_payload = payload
                else:
                    raw_body = request.body.decode('utf-8', errors='ignore') if request.body else ''
                    if raw_body:
                        body_payload = raw_body
        except Exception:
            body_payload = None

        query_payload = request.GET.dict() if request.GET else {}
        path_kwargs = getattr(getattr(request, 'resolver_match', None), 'kwargs', None) or {}

        combined_payload = {}
        if query_payload:
            combined_payload['query'] = query_payload
        if path_kwargs:
            combined_payload['path'] = path_kwargs
        if body_payload not in (None, '', {}):
            combined_payload['body'] = body_payload

        if combined_payload:
            return self._truncate_text(json.dumps(combined_payload, ensure_ascii=False, default=str))

        return ''

    @staticmethod
    def _extract_response_data(response):
        content = getattr(response, 'content', b'') or b''
        if not content:
            return ''
        try:
            payload = json.loads(content.decode('utf-8'))
        except Exception:
            return ''
        try:
            return json.dumps(payload, ensure_ascii=False, sort_keys=True)
        except Exception:
            return JwtAuthenticationMiddleware._truncate_text(content.decode('utf-8', errors='ignore'))

    @staticmethod
    def _get_client_ip(request):
        x_forwarded_for = request.META.get('HTTP_X_FORWARDED_FOR')
        if x_forwarded_for:
            return x_forwarded_for.split(',')[0].strip()
        return request.META.get('REMOTE_ADDR', '')

    @staticmethod
    def _extract_response_message(response):
        content = getattr(response, 'content', b'') or b''
        if not content:
            return ''
        try:
            payload = json.loads(content.decode('utf-8'))
        except Exception:
            return ''
        if isinstance(payload, dict):
            msg = payload.get('msg', '')
            if isinstance(msg, str):
                return msg[:255]
        return ''
