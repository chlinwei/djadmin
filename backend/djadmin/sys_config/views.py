from rest_framework import viewsets, status
from rest_framework.decorators import action
from rest_framework.response import Response
from sys_config.models import SysConfig
from sys_config.serializer import SysConfigSerializer


class SysConfigViewSet(viewsets.ModelViewSet):
    queryset = SysConfig.objects.all()
    serializer_class = SysConfigSerializer
    http_method_names = ['get', 'patch']  # 只允许查询和修改，不允许新增/删除

    def get_queryset(self):
        queryset = SysConfig.objects.all()
        search = self.request.query_params.get('search')
        if search:
            from django.db.models import Q
            queryset = queryset.filter(
                Q(name__icontains=search) | Q(key__icontains=search)
            )
        return queryset.order_by('key')

    def partial_update(self, request, *args, **kwargs):
        instance = self.get_object()
        if instance.is_readonly:
            return Response({'error': '该参数为只读，不可修改'}, status=status.HTTP_400_BAD_REQUEST)
        serializer = self.get_serializer(instance, data=request.data, partial=True)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response(serializer.data)

    @action(detail=False, methods=['get'], url_path='by-key/(?P<key>[^/]+)')
    def by_key(self, request, key=None):
        """通过 key 获取单个参数值"""
        try:
            config = SysConfig.objects.get(key=key)
            return Response({
                'key': config.key,
                'value': config.get_typed_value(),
                'name': config.name,
            })
        except SysConfig.DoesNotExist:
            return Response({'error': f'参数 {key} 不存在'}, status=status.HTTP_404_NOT_FOUND)

    @action(detail=False, methods=['patch'], url_path='update-by-key/(?P<key>[^/]+)')
    def update_by_key(self, request, key=None):
        """通过 key 更新参数值"""
        try:
            config = SysConfig.objects.get(key=key)
            if config.is_readonly:
                return Response({'error': '该参数为只读，不可修改'}, status=status.HTTP_400_BAD_REQUEST)
            config.value = str(request.data.get('value', config.value))
            config.save(update_fields=['value', 'update_time'])
            return Response(SysConfigSerializer(config).data)
        except SysConfig.DoesNotExist:
            return Response({'error': f'参数 {key} 不存在'}, status=status.HTTP_404_NOT_FOUND)
