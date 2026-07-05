from datetime import timezone as datetime_timezone
from io import BytesIO
import re
from urllib.parse import quote
from zipfile import ZIP_DEFLATED, ZipFile

from django.http import HttpResponse
from django.db.models import Q
from django.utils.dateparse import parse_datetime
from asgiref.sync import async_to_sync
from django_filters.rest_framework import DjangoFilterBackend
from django.utils import timezone
from rest_framework.filters import OrderingFilter, SearchFilter
from rest_framework.mixins import ListModelMixin
from rest_framework.viewsets import GenericViewSet
from rest_framework.decorators import action

from assets.models import WebSSHSessionLog
from assets.webssh_runtime import WebSSHRuntimeRegistry
from djadmin.utils import CustomPagination, Response_200
from menu.permisssion import CustomMenuPermission

from .models import LoginAuditLog, OperationAuditLog
from .serializer import (
    LoginAuditLogSerializer,
    OperationAuditLogSerializer,
    WebSSHSessionLogAuditSerializer,
    WebSSHSessionLogContentSerializer,
)


class WebSSHSessionLogAuditManage(GenericViewSet, ListModelMixin):
    queryset = WebSSHSessionLog.objects.select_related('host').order_by('-start_time')
    serializer_class = WebSSHSessionLogAuditSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    search_fields = ['username', 'host__name', 'host__instance_name', 'host__ip']
    ordering_fields = ['start_time', 'end_time', 'duration_seconds', 'command_count', 'input_bytes']
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'audit:webssh_sessions:view',
        'content': 'audit:webssh_sessions:view',
        'download': 'audit:webssh_sessions:view',
        'download_all': 'audit:webssh_sessions:view',
    }

    def get_queryset(self):
        queryset = super().get_queryset()
        status = self.request.query_params.get('status')  # type: ignore[union-attr]
        username = self.request.query_params.get('username')  # type: ignore[union-attr]
        keyword = self.request.query_params.get('keyword')  # type: ignore[union-attr]
        output_keyword = self.request.query_params.get('output_keyword')  # type: ignore[union-attr]
        start_time_from = self.request.query_params.get('start_time_from')  # type: ignore[union-attr]
        start_time_to = self.request.query_params.get('start_time_to')  # type: ignore[union-attr]
        active_session_ids = WebSSHRuntimeRegistry.get_active_session_ids()

        if status == WebSSHSessionLog.Status.CONNECTED:
            queryset = queryset.filter(
                status=WebSSHSessionLog.Status.CONNECTED,
                id__in=active_session_ids,
            )
        elif status == WebSSHSessionLog.Status.CLOSED:
            queryset = queryset.filter(
                Q(status=WebSSHSessionLog.Status.CLOSED)
                | (
                    Q(status=WebSSHSessionLog.Status.CONNECTED)
                    & ~Q(id__in=active_session_ids)
                )
            )
        elif status == WebSSHSessionLog.Status.FAILED:
            queryset = queryset.filter(status=status)
        if username:
            queryset = queryset.filter(username__icontains=username)
        if keyword:
            queryset = queryset.filter(
                Q(username__icontains=keyword)
                | Q(host__name__icontains=keyword)
                | Q(host__instance_name__icontains=keyword)
                | Q(host__ip__icontains=keyword)
            )
        if output_keyword:
            queryset = queryset.filter(output_content__icontains=output_keyword)

        start_from = self._parse_datetime_param(start_time_from)
        if start_from is not None:
            queryset = queryset.filter(start_time__gte=start_from)

        start_to = self._parse_datetime_param(start_time_to)
        if start_to is not None:
            queryset = queryset.filter(start_time__lte=start_to)

        return queryset

    @staticmethod
    def _parse_datetime_param(value):
        if not value:
            return None
        parsed = parse_datetime(value)
        if parsed is None:
            return None
        if timezone.is_naive(parsed):
            parsed = timezone.make_aware(parsed, datetime_timezone.utc)
        return parsed

    @staticmethod
    def _parse_list_param(query_params, name):
        values = []
        for raw in query_params.getlist(name):
            if not raw:
                continue
            values.extend([item.strip() for item in raw.split(',') if item.strip()])
        return values

    @staticmethod
    def _safe_filename_part(value, fallback='unknown'):
        text = str(value or '').strip()
        if not text:
            text = fallback
        return re.sub(r'[\\/:*?"<>|\r\n]+', '_', text)

    def _build_session_log_filename(self, instance):
        host_name = self._safe_filename_part(
            getattr(instance.host, 'instance_name', '') or getattr(instance.host, 'name', ''),
            'unknown-host',
        )
        host_ip = self._safe_filename_part(getattr(instance.host, 'ip', ''), 'unknown-ip')
        start_time = getattr(instance, 'start_time', None)
        start_time_text = 'unknown-start-time'
        if start_time is not None:
            start_time_text = timezone.localtime(start_time).strftime('%Y-%m-%d-%H-%M-%S')
        return f'webssh-{instance.id}-{host_name}({host_ip})-{start_time_text}.log'

    @staticmethod
    def _build_session_log_content(payload):
        lines = [
            'Web SSH Session Log',
            f"Status: {payload.get('status') or ''}",
            f"Start Time: {payload.get('start_time') or ''}",
            f"End Time: {payload.get('end_time') or ''}",
            f"Duration Seconds: {payload.get('duration_seconds') if payload.get('duration_seconds') is not None else ''}",
            f"Recorded Content Bytes: {payload.get('recorded_content_bytes') or 0}",
            f"Is Content Truncated: {bool(payload.get('is_content_truncated'))}",
            '',
            '=== Output ===',
            payload.get('output_content') or payload.get('raw_output_content') or '',
            '',
        ]
        return '\n'.join(lines)

    def list(self, request, *args, **kwargs):
        queryset = self.filter_queryset(self.get_queryset())
        page = self.paginate_queryset(queryset)
        serializer = self.get_serializer(page if page is not None else queryset, many=True)
        data = serializer.data
        if page is not None:
            paginator = self.paginator
            return Response_200(data={
                'count': paginator.page.paginator.count,
                'results': data,
                'pageNumber': paginator.page.number,
                'pageSize': paginator.page_size,
                'totalPages': paginator.page.paginator.num_pages,
                'next': paginator.get_next_link(),
                'previous': paginator.get_previous_link(),
            })
        return Response_200(data=data)

    @action(detail=True, methods=['get'], url_path='content')
    def content(self, request, pk=None):
        instance = self.get_object()
        async_to_sync(WebSSHRuntimeRegistry.flush_session_buffers)(instance.id)
        instance.refresh_from_db()
        serializer = WebSSHSessionLogContentSerializer(instance)
        return Response_200(data=serializer.data)

    @action(detail=True, methods=['get'], url_path='download')
    def download(self, request, pk=None):
        instance = self.get_object()
        async_to_sync(WebSSHRuntimeRegistry.flush_session_buffers)(instance.id)
        instance.refresh_from_db()
        serializer = WebSSHSessionLogContentSerializer(instance)
        payload = serializer.data

        content = self._build_session_log_content(payload)
        filename = self._build_session_log_filename(instance)
        response = HttpResponse(content, content_type='text/plain; charset=utf-8')
        response['Content-Disposition'] = f"attachment; filename*=UTF-8''{quote(filename)}"
        return response

    @action(detail=False, methods=['get'], url_path='download-all')
    def download_all(self, request):
        queryset = self.filter_queryset(self.get_queryset())

        selected_ids_raw = self._parse_list_param(request.query_params, 'ids')
        selected_ids = []
        for value in selected_ids_raw:
            try:
                selected_ids.append(int(value))
            except (TypeError, ValueError):
                continue

        if selected_ids:
            selected_filter = Q()
            if selected_ids:
                selected_filter |= Q(id__in=selected_ids)
            queryset = queryset.filter(selected_filter)

        entries = []
        for item in queryset:
            async_to_sync(WebSSHRuntimeRegistry.flush_session_buffers)(item.id)
            item.refresh_from_db()
            payload = WebSSHSessionLogContentSerializer(item).data
            entries.append((self._build_session_log_filename(item), self._build_session_log_content(payload)))

        if len(entries) == 1:
            file_name, content = entries[0]
            response = HttpResponse(content, content_type='text/plain; charset=utf-8')
            response['Content-Disposition'] = f"attachment; filename*=UTF-8''{quote(file_name)}"
            return response

        if len(entries) > 1:
            timestamp = timezone.now().strftime('%Y-%m-%d-%H-%M-%S')
            archive_name = f'webssh-{timestamp}.zip'
            zip_buffer = BytesIO()
            with ZipFile(zip_buffer, mode='w', compression=ZIP_DEFLATED) as zip_file:
                for file_name, content in entries:
                    zip_file.writestr(file_name, content)

            response = HttpResponse(zip_buffer.getvalue(), content_type='application/zip')
            response['Content-Disposition'] = f"attachment; filename*=UTF-8''{quote(archive_name)}"
            return response

        lines = [
            'Web SSH Session Logs',
            f'Total: {len(entries)}',
            '',
        ]
        for _, content in entries:
            lines.extend([
                '----------------------------------------',
                content,
            ])

        response = HttpResponse('\n'.join(lines), content_type='text/plain; charset=utf-8')
        response['Content-Disposition'] = "attachment; filename*=UTF-8''webssh-sessions.log"
        return response


class LoginAuditLogManage(GenericViewSet, ListModelMixin):
    queryset = LoginAuditLog.objects.all().order_by('-login_time')
    serializer_class = LoginAuditLogSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    search_fields = ['username', 'client_ip', 'message']
    ordering_fields = ['login_time']
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'audit:login_logs:view',
    }

    def get_queryset(self):
        queryset = super().get_queryset()
        status = self.request.query_params.get('status')  # type: ignore[union-attr]
        keyword = self.request.query_params.get('keyword')  # type: ignore[union-attr]
        login_time_from = self.request.query_params.get('login_time_from')  # type: ignore[union-attr]
        login_time_to = self.request.query_params.get('login_time_to')  # type: ignore[union-attr]

        if status in {LoginAuditLog.Status.SUCCESS, LoginAuditLog.Status.FAILED}:
            queryset = queryset.filter(status=status)
        if keyword:
            queryset = queryset.filter(
                Q(username__icontains=keyword)
                | Q(client_ip__icontains=keyword)
                | Q(message__icontains=keyword)
            )

        time_from = self._parse_datetime_param(login_time_from)
        if time_from is not None:
            queryset = queryset.filter(login_time__gte=time_from)

        time_to = self._parse_datetime_param(login_time_to)
        if time_to is not None:
            queryset = queryset.filter(login_time__lte=time_to)

        return queryset

    @staticmethod
    def _parse_datetime_param(value):
        if not value:
            return None
        parsed = parse_datetime(value)
        if parsed is None:
            return None
        if timezone.is_naive(parsed):
            parsed = timezone.make_aware(parsed, datetime_timezone.utc)
        return parsed

    def list(self, request, *args, **kwargs):
        queryset = self.filter_queryset(self.get_queryset())
        page = self.paginate_queryset(queryset)
        serializer = self.get_serializer(page if page is not None else queryset, many=True)
        data = serializer.data
        if page is not None:
            paginator = self.paginator
            return Response_200(data={
                'count': paginator.page.paginator.count,
                'results': data,
                'pageNumber': paginator.page.number,
                'pageSize': paginator.page_size,
                'totalPages': paginator.page.paginator.num_pages,
                'next': paginator.get_next_link(),
                'previous': paginator.get_previous_link(),
            })
        return Response_200(data=data)


class OperationAuditLogManage(GenericViewSet, ListModelMixin):
    queryset = OperationAuditLog.objects.exclude(method='GET').order_by('-created_at')
    serializer_class = OperationAuditLogSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    search_fields = ['username', 'method', 'path', 'route_name', 'client_ip', 'message']
    ordering_fields = ['created_at', 'duration_ms', 'status_code']
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'audit:operation_logs:view',
    }

    def get_queryset(self):
        queryset = super().get_queryset()
        keyword = self.request.query_params.get('keyword')  # type: ignore[union-attr]
        method = self.request.query_params.get('method')  # type: ignore[union-attr]
        status_code = self.request.query_params.get('status_code')  # type: ignore[union-attr]
        created_at_from = self.request.query_params.get('created_at_from')  # type: ignore[union-attr]
        created_at_to = self.request.query_params.get('created_at_to')  # type: ignore[union-attr]

        if keyword:
            queryset = queryset.filter(
                Q(username__icontains=keyword)
                | Q(method__icontains=keyword)
                | Q(path__icontains=keyword)
                | Q(route_name__icontains=keyword)
                | Q(client_ip__icontains=keyword)
                | Q(message__icontains=keyword)
            )
        if method:
            queryset = queryset.filter(method__iexact=method)
        if status_code:
            queryset = queryset.filter(status_code=status_code)

        created_from = self._parse_datetime_param(created_at_from)
        if created_from is not None:
            queryset = queryset.filter(created_at__gte=created_from)

        created_to = self._parse_datetime_param(created_at_to)
        if created_to is not None:
            queryset = queryset.filter(created_at__lte=created_to)

        return queryset

    @staticmethod
    def _parse_datetime_param(value):
        if not value:
            return None
        parsed = parse_datetime(value)
        if parsed is None:
            return None
        if timezone.is_naive(parsed):
            parsed = timezone.make_aware(parsed, datetime_timezone.utc)
        return parsed

    def list(self, request, *args, **kwargs):
        queryset = self.filter_queryset(self.get_queryset())
        page = self.paginate_queryset(queryset)
        serializer = self.get_serializer(page if page is not None else queryset, many=True)
        data = serializer.data
        if page is not None:
            paginator = self.paginator
            return Response_200(data={
                'count': paginator.page.paginator.count,
                'results': data,
                'pageNumber': paginator.page.number,
                'pageSize': paginator.page_size,
                'totalPages': paginator.page.paginator.num_pages,
                'next': paginator.get_next_link(),
                'previous': paginator.get_previous_link(),
            })
        return Response_200(data=data)
