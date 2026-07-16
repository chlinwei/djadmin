from __future__ import annotations

from .view_helpers import *
from .view_helpers import _apply_limit_to_inventory_snapshot, _build_limit_matched_hosts_preview

class AutomationInventoryManage(GenericViewSet, CreateModelMixin, UpdateModelMixin, RetrieveModelMixin, ListModelMixin, DestroyModelMixin):
    queryset = AutomationInventory.objects.all()
    serializer_class = AutomationInventorySerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    search_fields = ['name', 'remark']
    ordering_fields = ['name', 'create_time', 'update_time']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'automation:inventory:view',
        'retrieve': 'automation:inventory:view',
        'create': 'automation:inventory:create',
        'destroy': 'automation:inventory:delete',
        'partial_update': 'automation:inventory:update',
        'perform_update': 'automation:inventory:update',
        'precheck_limit': 'automation:inventory:view',
    }

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

    def retrieve(self, request, *args, **kwargs):
        instance = self.get_object()
        serializer = self.get_serializer(instance)
        return Response_200(data=serializer.data)

    def create(self, request, *args, **kwargs):
        serializer = self.get_serializer(data=request.data)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    def update(self, request, *args, **kwargs):
        instance = self.get_object()
        serializer = self.get_serializer(instance, data=request.data, partial=False)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    def partial_update(self, request, *args, **kwargs):
        instance = self.get_object()
        serializer = self.get_serializer(instance, data=request.data, partial=True)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    def destroy(self, request, *args, **kwargs):
        instance = self.get_object()
        deleted_id = instance.id
        self.perform_destroy(instance)
        return Response_200(data={'id': deleted_id})

    @action(detail=True, methods=['post'], url_path='precheck-limit')
    def precheck_limit(self, request, id=None):
        inventory = self.get_object()

        if not inventory.enabled:
            return Response_200(data={
                'ok': False,
                'status': 'inventory_disabled',
                'message': 'Inventory 已禁用，无法执行预检',
                'resolved_host_count': 0,
                'effective_limit': '',
                'matched_hosts_preview': [],
                'matched_hosts_preview_total': 0,
            })

        limit_text = str(request.data.get('limit', '')).strip()
        host_ids_raw = request.data.get('host_ids', inventory.selected_host_ids)
        group_ids_raw = request.data.get('group_ids', inventory.selected_group_ids)

        host_ids = host_ids_raw if isinstance(host_ids_raw, list) else []
        group_ids = group_ids_raw if isinstance(group_ids_raw, list) else []
        host_ids = [int(item) for item in host_ids if str(item).isdigit()]
        group_ids = [int(item) for item in group_ids if str(item).isdigit()]

        existing_group_ids = set(HostGroup.objects.filter(id__in=group_ids).values_list('id', flat=True))
        missing_group_ids = sorted(set(group_ids) - existing_group_ids)
        if missing_group_ids:
            return Response_200(data={
                'ok': False,
                'status': 'inventory_invalid',
                'message': f'执行范围包含已删除主机组: {", ".join(str(item) for item in missing_group_ids)}',
                'resolved_host_count': 0,
                'effective_limit': limit_text,
                'matched_hosts_preview': [],
                'matched_hosts_preview_total': 0,
                'missing_group_ids': missing_group_ids,
            })

        inventory_snapshot = build_inventory_snapshot(host_ids=host_ids, group_ids=group_ids)
        inventory_snapshot = _apply_limit_to_inventory_snapshot(inventory_snapshot, limit_text)
        hosts = inventory_snapshot.get('hosts', []) if isinstance(inventory_snapshot, dict) else []
        resolved_host_count = len(hosts) if isinstance(hosts, list) else 0

        if resolved_host_count == 0:
            return Response_200(data={
                'ok': False,
                'status': 'inventory_empty',
                'message': f'Inventory [{inventory.name or "-"}] 当前无匹配主机',
                'resolved_host_count': 0,
                'effective_limit': limit_text,
                'matched_hosts_preview': [],
                'matched_hosts_preview_total': 0,
            })

        return Response_200(data={
            'ok': True,
            'status': 'ok',
            'message': f'预检通过，可匹配主机 {resolved_host_count} 台',
            'resolved_host_count': resolved_host_count,
            'effective_limit': limit_text,
            'matched_hosts_preview': _build_limit_matched_hosts_preview(inventory_snapshot),
            'matched_hosts_preview_total': resolved_host_count,
        })


