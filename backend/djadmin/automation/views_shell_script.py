from __future__ import annotations

import shutil
import subprocess

from .view_helpers import *


class ShellScriptTemplateManage(GenericViewSet, CreateModelMixin, UpdateModelMixin, RetrieveModelMixin, ListModelMixin, DestroyModelMixin):
    queryset = ShellScriptTemplate.objects.all()
    serializer_class = ShellScriptTemplateSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    search_fields = ['name', 'description', 'remark']
    ordering_fields = ['name', 'create_time', 'update_time']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'automation:shell_scripts:view',
        'retrieve': 'automation:shell_scripts:view',
        'validate_content': 'automation:shell_scripts:view',
        'create': 'automation:shell_scripts:create',
        'destroy': 'automation:shell_scripts:delete',
        'partial_update': 'automation:shell_scripts:update',
        'perform_update': 'automation:shell_scripts:update',
        'upload_file': 'automation:shell_scripts:update',
        'download_file': 'automation:shell_scripts:view',
    }

    @staticmethod
    def _build_download_filename(template: ShellScriptTemplate) -> str:
        raw_name = template.name or f'shell-script-{template.pk}'
        sanitized = re.sub(r'[^A-Za-z0-9._-]+', '-', raw_name).strip('-')
        return f'{sanitized or "shell-script-template"}.sh'

    def get_queryset(self):
        # 可选 category 过滤：前端“模板”列表页默认只看“通用”分类，
        # 软件包安装/卸载专用脚本模板需要显式切换筛选才会出现。
        queryset = super().get_queryset()
        category = (self.request.query_params.get('category') or '').strip()  # type: ignore[union-attr]
        if category:
            queryset = queryset.filter(category=category)
        return queryset

    def perform_update(self, serializer):
        # 监控软件仓库的安装/卸载固定只能绑 PlaybookTemplate（见 monitor.SoftwarePackage），
        # Shell 脚本模板不可能被它引用，因此无需降级保护。
        serializer.save()

    def perform_create(self, serializer):
        serializer.save()

    @action(detail=False, methods=['post'], url_path='validate')
    def validate_content(self, request):
        content = str(request.data.get('content', '') or '')
        if not content.strip():
            return Response_error_str('Shell script content is empty', code=400)

        checker = shutil.which('bash') or shutil.which('sh')
        if not checker:
            return Response_error_str('Shell syntax checker is unavailable (bash/sh not found)', code=500)

        result = subprocess.run(
            [checker, '-n'],
            input=content,
            text=True,
            capture_output=True,
            check=False,
        )
        if result.returncode != 0:
            error_text = (result.stderr or result.stdout or 'Shell syntax error').strip()
            return Response_error_str(error_text, code=400)
        return Response_200(data={'valid': True})

    @action(detail=True, methods=['post'], url_path='upload')
    def upload_file(self, request, id=None):
        template = self.get_object()
        uploaded_file = request.FILES.get('file')
        if uploaded_file is None:
            return Response_error_str('Please upload a shell script file', code=400)

        suffix = os.path.splitext(uploaded_file.name or '')[1].lower()
        if suffix != '.sh':
            return Response_error_str('Only .sh files are supported', code=400)

        try:
            content = uploaded_file.read().decode('utf-8')
        except UnicodeDecodeError:
            return Response_error_str('Script file must be UTF-8 encoded', code=400)

        if not content.strip():
            return Response_error_str('Script file is empty', code=400)

        template.content = content
        template.save(update_fields=['content', 'update_time'])

        serializer = self.get_serializer(template)
        return Response_200(data=serializer.data)

    @action(detail=True, methods=['get'], url_path='download')
    def download_file(self, request, id=None):
        template = self.get_object()
        filename = self._build_download_filename(template)
        response = HttpResponse(template.content or '', content_type='text/x-shellscript; charset=utf-8')
        response['Content-Disposition'] = f"attachment; filename*=UTF-8''{quote(filename)}"
        return response
