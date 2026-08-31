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

        host_ids = host_ids_raw if isinstance(host_ids_raw, list) else []
        host_ids = [int(item) for item in host_ids if str(item).isdigit()]

        inventory_snapshot = build_inventory_snapshot(host_ids=host_ids)
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


