import json

from django.http import JsonResponse
from django.utils.deprecation import MiddlewareMixin
from django.utils import timezone
from jwt import ExpiredSignatureError, InvalidTokenError, PyJWTError
from rest_framework_jwt.settings import api_settings

from user.utils import getCurrentUser


class JwtAuthenticationMiddleware(MiddlewareMixin):

    _AUDIT_SKIP_PREFIXES = ('/sys/audit/', '/media', '/static')
    _AUDIT_SKIP_PATHS = {'/sys/login', '/sys/login2'}

    def process_request(self, request):
        request._operation_audit_started_at = timezone.now()
        request._operation_audit_request_data = self._extract_request_data(request)
        white_list = ["/sys/login"]  # 请求白名单
        path = request.path
        if path not in white_list and not path.startswith("/media"):
            token = request.META.get('HTTP_AUTHORIZATION')
            err_ret = {
                'code':301,
                'msg': ''
            }
            try:
                jwt_decode_handler = api_settings.JWT_DECODE_HANDLER
                if not callable(jwt_decode_handler):
                    raise InvalidTokenError()
                jwt_decode_handler(token)
                
            except ExpiredSignatureError:
                err_ret['msg'] = 'Token过期，请重新登录！'
                return JsonResponse(err_ret)
            except InvalidTokenError:
                err_ret['msg'] = 'Token验证失败！'
                return JsonResponse(err_ret)
            except PyJWTError:
                err_ret['msg'] = 'Token验证异常！'
                return JsonResponse(err_ret)
        else:
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
        try:
            if 'application/json' in content_type:
                raw_body = request.body.decode('utf-8') if request.body else ''
                if raw_body:
                    parsed_body = json.loads(raw_body)
                    return self._truncate_text(json.dumps(parsed_body, ensure_ascii=False, default=str))
                return ''
            if 'multipart/form-data' in content_type:
                payload = request.POST.dict()
                if payload:
                    return self._truncate_text(json.dumps(payload, ensure_ascii=False, default=str))
                return ''
            payload = request.POST.dict()
            if payload:
                return self._truncate_text(json.dumps(payload, ensure_ascii=False, default=str))
            raw_body = request.body.decode('utf-8', errors='ignore') if request.body else ''
            return self._truncate_text(raw_body)
        except Exception:
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
