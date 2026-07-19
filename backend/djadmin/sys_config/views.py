from rest_framework import viewsets, status
from rest_framework.decorators import action
from rest_framework.decorators import api_view
from rest_framework.response import Response
from sys_config.models import SysConfig
from sys_config.serializer import SysConfigSerializer
from user.utils import getCurrentUser
from djadmin.utils import CustomPagination, Response_200, Response_error_str
import json

from scheduler_manager import ensure_scheduler_log_configs


AGENT_HOST_COLLECT_INTERVAL_SECONDS_KEY = 'sys.assets.collect.interval_seconds'
AGENT_CONFIG_EXPOSED_KEYS = {AGENT_HOST_COLLECT_INTERVAL_SECONDS_KEY}
AGENT_HOST_COLLECT_INTERVAL_SECONDS_MIN = 30
AGENT_HOST_COLLECT_INTERVAL_SECONDS_MAX = 12 * 60 * 60
HOST_MANAGE_REFRESH_INTERVAL_SECONDS_KEY = 'sys.assets.host.manage.refresh_interval_seconds'
HOST_DETAIL_COLLECT_DISPATCH_INTERVAL_SECONDS_KEY = 'sys.assets.host.detail.collect_dispatch_interval_seconds'
AUTOMATION_LOGS_REFRESH_INTERVAL_SECONDS_KEY = 'sys.automation.logs.refresh_interval_seconds'
AUTOMATION_WS_JOB_LOG_POLL_INTERVAL_SECONDS_KEY = 'sys.automation.websocket.job_log_poll_interval_seconds'
AUTOMATION_WS_WORKFLOW_RUN_POLL_INTERVAL_SECONDS_KEY = 'sys.automation.websocket.workflow_run_poll_interval_seconds'


def ensure_agent_collect_configs():
    SysConfig.objects.get_or_create(
        key=AGENT_HOST_COLLECT_INTERVAL_SECONDS_KEY,
        defaults={
            'value': '40',
            'default_value': '40',
            'value_type': 'int',
            'name': '主机信息采集间隔（秒）',
            'description': 'Agent 主机信息周期上报间隔（秒）',
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


@api_view(['GET'])
def agent_config_by_key(request, key=None):
    # Agent 启动只允许读取白名单参数，避免暴露全量系统配置。
    ensure_agent_collect_configs()
    if key not in AGENT_CONFIG_EXPOSED_KEYS:
        return Response_error_str(f'参数 {key} 不存在', code=404)

    try:
        config = SysConfig.objects.get(key=key)
    except SysConfig.DoesNotExist:
        return Response_error_str(f'参数 {key} 不存在', code=404)

    return Response_200({
        'key': config.key,
        'value': config.get_typed_value(),
        'name': config.name,
    })


class SysConfigViewSet(viewsets.ModelViewSet):
    queryset = SysConfig.objects.all()
    serializer_class = SysConfigSerializer
    pagination_class = CustomPagination
    http_method_names = ['get', 'patch', 'post']  # 只允许查询和修改，不允许新增/删除

    def get_queryset(self):
        # Ensure scheduler-related default config keys are present even if user opens
        # system config page before visiting scheduler task center.
        ensure_scheduler_log_configs()
        ensure_agent_collect_configs()
        ensure_host_manage_refresh_interval_config()
        ensure_host_detail_collect_dispatch_interval_config()
        ensure_automation_logs_refresh_interval_config()
        ensure_automation_ws_job_log_poll_interval_config()
        ensure_automation_ws_workflow_run_poll_interval_config()
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

        mutable_data = request.data.copy()
        mutable_data['value'] = normalized_value
        if 'default_value' in request.data:
            mutable_data['default_value'] = normalized_default

        serializer = self.get_serializer(instance, data=mutable_data, partial=True)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        dispatch_result = self._try_dispatch_agent_collect_interval_update(
            key=instance.key,
            previous_value=instance.value,
            next_value=normalized_value,
        )
        if dispatch_result is not None and dispatch_result.get('ok') is False:
            return Response_error_str(
                dispatch_result.get('error') or '参数已更新，但触发 agent 失败',
                code=500,
                data=serializer.data,
            )
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
        dispatch_result = self._try_dispatch_agent_collect_interval_update(
            key=config.key,
            previous_value='',
            next_value=config.value,
        )
        if dispatch_result is not None and dispatch_result.get('ok') is False:
            return Response_error_str(
                dispatch_result.get('error') or '参数已更新，但触发 agent 失败',
                code=500,
                data=SysConfigSerializer(config).data,
            )
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
                previous_value = config.value
                config.value = self._normalize_value_by_type(
                    request.data.get('value', config.value),
                    config.value_type,
                    key=config.key,
                )
            except ValueError as exc:
                return Response_error_str(str(exc), code=400)

            config.save(update_fields=['value', 'update_time'])
            dispatch_result = self._try_dispatch_agent_collect_interval_update(
                key=config.key,
                previous_value=previous_value,
                next_value=config.value,
            )
            if dispatch_result is not None and dispatch_result.get('ok') is False:
                return Response_error_str(
                    dispatch_result.get('error') or '参数已更新，但触发 agent 失败',
                    code=500,
                    data=SysConfigSerializer(config).data,
                )
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

            if key == AGENT_HOST_COLLECT_INTERVAL_SECONDS_KEY:
                if normalized < AGENT_HOST_COLLECT_INTERVAL_SECONDS_MIN:
                    raise ValueError(f'主机信息采集间隔最小为 {AGENT_HOST_COLLECT_INTERVAL_SECONDS_MIN} 秒')
                if normalized > AGENT_HOST_COLLECT_INTERVAL_SECONDS_MAX:
                    raise ValueError(f'主机信息采集间隔最大为 {AGENT_HOST_COLLECT_INTERVAL_SECONDS_MAX} 秒')

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

    def _try_dispatch_agent_collect_interval_update(self, key, previous_value, next_value):
        if key != AGENT_HOST_COLLECT_INTERVAL_SECONDS_KEY:
            return None
        if str(previous_value) == str(next_value):
            return {
                'ok': True,
                'dispatch': None,
            }

        try:
            from assets.views import dispatch_host_report_interval_update

            dispatch_data = dispatch_host_report_interval_update(
                interval_seconds=int(str(next_value)),
            )
            return {
                'ok': True,
                'dispatch': dispatch_data,
            }
        except Exception as exc:
            return {
                'ok': False,
                'error': f'参数已更新，但触发 agent 任务失败：{exc}',
            }
