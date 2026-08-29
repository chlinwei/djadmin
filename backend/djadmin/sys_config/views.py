from rest_framework import viewsets
from rest_framework.decorators import action
from django.contrib.auth.hashers import make_password
from sys_config.models import SECRET_MASK_PLACEHOLDER, SysConfig
from sys_config.serializer import SysConfigSerializer
from user.utils import getCurrentUser
from djadmin.utils import CustomPagination, Response_200, Response_error_str
import json

from scheduler_manager import ensure_scheduler_log_configs


HOST_MANAGE_REFRESH_INTERVAL_SECONDS_KEY = 'sys.assets.host.manage.refresh_interval_seconds'
AGENT_GRPC_ADVERTISE_ADDR_KEY = 'sys.assets.agent.grpc_advertise_addr'
HOST_DETAIL_COLLECT_DISPATCH_INTERVAL_SECONDS_KEY = 'sys.assets.host.detail.collect_dispatch_interval_seconds'
AUTOMATION_LOGS_REFRESH_INTERVAL_SECONDS_KEY = 'sys.automation.logs.refresh_interval_seconds'
AUTOMATION_WS_JOB_LOG_POLL_INTERVAL_SECONDS_KEY = 'sys.automation.websocket.job_log_poll_interval_seconds'
AUTOMATION_WS_WORKFLOW_RUN_POLL_INTERVAL_SECONDS_KEY = 'sys.automation.websocket.workflow_run_poll_interval_seconds'
ALERT_HISTORY_RETENTION_DAYS_KEY = 'sys.monitor.alert_history.retention_days'


def ensure_alert_history_retention_config():
    SysConfig.objects.get_or_create(
        key=ALERT_HISTORY_RETENTION_DAYS_KEY,
        defaults={
            'value': '90',
            'default_value': '90',
            'value_type': 'int',
            'name': '历史告警保留天数',
            'description': '只清理超过保留天数的已恢复告警，未恢复告警不按期限清理',
            'is_readonly': False,
        },
    )


def ensure_host_manage_refresh_interval_config():
    SysConfig.objects.get_or_create(
        key=HOST_MANAGE_REFRESH_INTERVAL_SECONDS_KEY,
        defaults={
            'value': '5',
            'default_value': '5',
            'value_type': 'int',
            'name': '主机管理页刷新间隔（秒）',
            'description': '主机管理列表自动刷新间隔（秒）',
            'is_readonly': False,
        },
    )


def ensure_agent_grpc_advertise_addr_config():
    SysConfig.objects.get_or_create(
        key=AGENT_GRPC_ADVERTISE_ADDR_KEY,
        defaults={
            'value': '',
            'default_value': '',
            'value_type': 'string',
            'name': 'Agent gRPC 对外地址',
            'description': 'Agent 连接 djadmin 后端的地址，例如 10.25.66.150:9001，不能填写 127.0.0.1 或 0.0.0.0',
            'is_readonly': False,
        },
    )


def ensure_host_detail_collect_dispatch_interval_config():
    SysConfig.objects.get_or_create(
        key=HOST_DETAIL_COLLECT_DISPATCH_INTERVAL_SECONDS_KEY,
        defaults={
            'value': '8',
            'default_value': '8',
            'value_type': 'int',
            'name': '主机详情主动采集下发间隔（秒）',
            'description': '主机详情页主动下发 dj-agent 采集任务的间隔（秒）',
            'is_readonly': False,
        },
    )


def ensure_automation_logs_refresh_interval_config():
    SysConfig.objects.get_or_create(
        key=AUTOMATION_LOGS_REFRESH_INTERVAL_SECONDS_KEY,
        defaults={
            'value': '5',
            'default_value': '5',
            'value_type': 'int',
            'name': '运行记录中心刷新间隔（秒）',
            'description': '自动化运行记录中心列表自动刷新间隔（秒）',
            'is_readonly': False,
        },
    )


def ensure_automation_ws_job_log_poll_interval_config():
    SysConfig.objects.get_or_create(
        key=AUTOMATION_WS_JOB_LOG_POLL_INTERVAL_SECONDS_KEY,
        defaults={
            'value': '0.5',
            'default_value': '0.5',
            'value_type': 'string',
            'name': '自动化作业日志WS轮询间隔（秒）',
            'description': '自动化作业日志 WebSocket 拉取后端增量的轮询间隔（秒）',
            'is_readonly': False,
        },
    )


def ensure_automation_ws_workflow_run_poll_interval_config():
    SysConfig.objects.get_or_create(
        key=AUTOMATION_WS_WORKFLOW_RUN_POLL_INTERVAL_SECONDS_KEY,
        defaults={
            'value': '0.5',
            'default_value': '0.5',
            'value_type': 'string',
            'name': '工作流运行状态WS轮询间隔（秒）',
            'description': '工作流运行状态 WebSocket 拉取后端状态的轮询间隔（秒）',
            'is_readonly': False,
        },
    )


class SysConfigViewSet(viewsets.ModelViewSet):
    queryset = SysConfig.objects.all()
    serializer_class = SysConfigSerializer
    pagination_class = CustomPagination
    http_method_names = ['get', 'patch', 'post']  # 只允许查询和修改，不允许新增/删除

    def get_queryset(self):
        # Ensure scheduler-related default config keys are present even if user opens
        # system config page before visiting scheduler task center.
        ensure_scheduler_log_configs()
        ensure_host_manage_refresh_interval_config()
        ensure_agent_grpc_advertise_addr_config()
        ensure_host_detail_collect_dispatch_interval_config()
        ensure_automation_logs_refresh_interval_config()
        ensure_automation_ws_job_log_poll_interval_config()
        ensure_automation_ws_workflow_run_poll_interval_config()
        ensure_alert_history_retention_config()
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
            normalized_value = self._normalize_value_by_type(value, instance.value_type, key=instance.key)
            normalized_default = None if default_value is None else self._normalize_value_by_type(
                default_value,
                instance.value_type,
                key=instance.key,
            )
        except ValueError as exc:
            return Response_error_str(str(exc), code=400)

        if instance.value_type == 'secret':
            # 前端展示的是掩码占位符：没传 value 或原样把占位符传回来都视为“未修改”，
            # 只有传入真正的新明文才重新哈希落库，避免把占位符字符串误当新密文存进去。
            if 'value' not in request.data or normalized_value == SECRET_MASK_PLACEHOLDER:
                normalized_value = instance.value
            else:
                normalized_value = make_password(normalized_value)

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

        # secret 类型的 default_value 是明文占位符（如 REPLACE_ME），落回 value 时必须重新哈希，
        # 不能像其他类型那样直接把明文字符串搬进 value 字段。
        if config.value_type == 'secret':
            config.value = make_password(str(config.default_value))
        else:
            config.value = str(config.default_value)
        config.save(update_fields=['value', 'update_time'])
        return Response_200(SysConfigSerializer(config).data)

    @action(detail=False, methods=['get'], url_path='by-key/(?P<key>[^/]+)')
    def by_key(self, request, key=None):
        """通过 key 获取单个参数值"""
        if key == ALERT_HISTORY_RETENTION_DAYS_KEY:
            ensure_alert_history_retention_config()
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
        if key == ALERT_HISTORY_RETENTION_DAYS_KEY:
            ensure_alert_history_retention_config()
        try:
            config = SysConfig.objects.get(key=key)
            if config.is_readonly:
                return Response_error_str('该参数为只读，不可修改', code=400)

            try:
                incoming_value = self._normalize_value_by_type(
                    request.data.get('value', config.value),
                    config.value_type,
                    key=config.key,
                )
                if config.value_type == 'secret':
                    if 'value' not in request.data or incoming_value == SECRET_MASK_PLACEHOLDER:
                        incoming_value = config.value
                    else:
                        incoming_value = make_password(incoming_value)
                config.value = incoming_value
            except ValueError as exc:
                return Response_error_str(str(exc), code=400)

            config.save(update_fields=['value', 'update_time'])
            return Response_200(SysConfigSerializer(config).data)
        except SysConfig.DoesNotExist:
            return Response_error_str(f'参数 {key} 不存在', code=404)

    def _is_admin(self, request):
        user_info = getCurrentUser(request)
        return bool(user_info and user_info.get('username') == 'admin')

    def _normalize_value_by_type(self, value, value_type, key=''):
        if value_type == 'int':
            try:
                normalized = int(str(value).strip())
            except (ValueError, TypeError):
                raise ValueError('参数值必须是整数')

            if key == ALERT_HISTORY_RETENTION_DAYS_KEY and normalized < 1:
                raise ValueError('历史告警保留天数不能小于 1 天')
            return str(normalized)

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
