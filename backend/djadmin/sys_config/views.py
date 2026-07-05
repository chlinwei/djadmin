from rest_framework import viewsets, status
from rest_framework.decorators import action
from rest_framework.response import Response
from sys_config.models import SysConfig
from sys_config.serializer import SysConfigSerializer
from user.utils import getCurrentUser
from djadmin.utils import CustomPagination, Response_200, Response_error_str
import json

from scheduler_manager import ensure_scheduler_log_configs


class SysConfigViewSet(viewsets.ModelViewSet):
    queryset = SysConfig.objects.all()
    serializer_class = SysConfigSerializer
    pagination_class = CustomPagination
    http_method_names = ['get', 'patch', 'post']  # 只允许查询和修改，不允许新增/删除

    def get_queryset(self):
        # Ensure scheduler-related default config keys are present even if user opens
        # system config page before visiting scheduler task center.
        ensure_scheduler_log_configs()
        queryset = SysConfig.objects.all()
        search = self.request.query_params.get('search')  # type: ignore[union-attr]
        if search:
            from django.db.models import Q
            queryset = queryset.filter(
                Q(name__icontains=search) | Q(key__icontains=search)
            )
        return queryset.order_by('key')

    def partial_update(self, request, *args, **kwargs):
        instance = self.get_object()
        if instance.is_readonly:
            return Response_error_str('该参数为只读，不可修改', code=400)

        # default_value modification is admin-only.
        if 'default_value' in request.data and not self._is_admin(request):
            return Response_error_str('仅管理员可修改默认值', code=403)

        value = request.data.get('value', instance.value)
        default_value = request.data.get('default_value', instance.default_value)
        try:
            normalized_value = self._normalize_value_by_type(value, instance.value_type)
            normalized_default = None if default_value is None else self._normalize_value_by_type(default_value, instance.value_type)
        except ValueError as exc:
            return Response_error_str(str(exc), code=400)

        mutable_data = request.data.copy()
        mutable_data['value'] = normalized_value
        if 'default_value' in request.data:
            mutable_data['default_value'] = normalized_default

        serializer = self.get_serializer(instance, data=mutable_data, partial=True)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(serializer.data)

    @action(detail=True, methods=['post'], url_path='reset-default')
    def reset_default(self, request, pk=None):
        """重置参数值为默认值"""
        config = self.get_object()
        if config.is_readonly:
            return Response_error_str('该参数为只读，不可重置', code=400)

        if config.default_value is None:
            return Response_error_str('该参数未配置默认值，无法重置', code=400)

        config.value = str(config.default_value)
        config.save(update_fields=['value', 'update_time'])
        return Response_200(SysConfigSerializer(config).data)

    @action(detail=False, methods=['get'], url_path='by-key/(?P<key>[^/]+)')
    def by_key(self, request, key=None):
        """通过 key 获取单个参数值"""
        try:
            config = SysConfig.objects.get(key=key)
            return Response_200({
                'key': config.key,
                'value': config.get_typed_value(),
                'name': config.name,
            })
        except SysConfig.DoesNotExist:
            return Response_error_str(f'参数 {key} 不存在', code=404)

    @action(detail=False, methods=['patch'], url_path='update-by-key/(?P<key>[^/]+)')
    def update_by_key(self, request, key=None):
        """通过 key 更新参数值"""
        try:
            config = SysConfig.objects.get(key=key)
            if config.is_readonly:
                return Response_error_str('该参数为只读，不可修改', code=400)

            try:
                config.value = self._normalize_value_by_type(request.data.get('value', config.value), config.value_type)
            except ValueError as exc:
                return Response_error_str(str(exc), code=400)

            config.save(update_fields=['value', 'update_time'])
            return Response_200(SysConfigSerializer(config).data)
        except SysConfig.DoesNotExist:
            return Response_error_str(f'参数 {key} 不存在', code=404)

    def _is_admin(self, request):
        user_info = getCurrentUser(request)
        return bool(user_info and user_info.get('username') == 'admin')

    def _normalize_value_by_type(self, value, value_type):
        if value_type == 'int':
            try:
                return str(int(str(value).strip()))
            except (ValueError, TypeError):
                raise ValueError('参数值必须是整数')

        if value_type == 'bool':
            text = str(value).strip().lower()
            if text in ('true', '1', 'yes', 'y'):
                return 'true'
            if text in ('false', '0', 'no', 'n'):
                return 'false'
            raise ValueError('参数值必须是布尔值（true/false）')

        if value_type == 'json':
            try:
                return json.dumps(json.loads(str(value)), ensure_ascii=False)
            except (ValueError, TypeError):
                raise ValueError('参数值必须是合法 JSON')

        # string or unknown type
        return '' if value is None else str(value)
