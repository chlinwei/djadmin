from django.shortcuts import render
from djadmin.utils import Response_200
from rest_framework.mixins import CreateModelMixin,DestroyModelMixin,UpdateModelMixin,RetrieveModelMixin,ListModelMixin
from rest_framework.viewsets import GenericViewSet
from rest_framework.decorators import action
from .models import *
from .serializer import *
from djadmin.utils import CustomPagination
from rest_framework.filters import OrderingFilter,SearchFilter
from django_filters.rest_framework  import DjangoFilterBackend
from djadmin.errordict import DjadminException,AssetsError
from io import TextIOWrapper
import csv
from menu.permisssion import CustomMenuPermission
from django.db.models import Prefetch


class CredentialManage(GenericViewSet,CreateModelMixin,UpdateModelMixin,RetrieveModelMixin,ListModelMixin):
    queryset =  Credential.objects.all()
    serializer_class = CredentialSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter,DjangoFilterBackend,SearchFilter)
    search_fields = ['name', 'remark'] 
    ordering_fields = [ 'name','create_time'] 
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        # view
        'list': 'assets:credentials:view',
        'retrieve': 'assets:credentials:view',
        # delete
        'batch-delete': 'assets:credentials:delete',
        # update
        'partial_update': 'assets:credentials:update',
        'perform_update': 'assets:credentials:update',
        # create
        'create': 'assets:credentials:create',
        'batch-create': 'assets:credentials:create',
    }
    # 批量删除credential
    @action(detail=False,methods=['delete'],url_path='batch-delete')
    def batchDelete(self,request):
        # 获取ID数组参数
        ids = request.data.get('ids', [])
        # 先查用户角色列表
        Credential.objects.filter(id__in=ids).delete()
        return Response_200()
    # 批量导入credential
    @action(detail=False,methods=['post'],url_path='batch-create')
    def batchCreate(self,request):
        file = request.FILES.get('file')
        if not file.name.endswith('csv'):
            raise DjadminException(AssetsError.FILE_NOT_ENDSWITH_CSV)
        decoded_file = TextIOWrapper(file.file, encoding='utf-8')
        reader = csv.DictReader(decoded_file)
        input_datas = []
        for row in reader:
            input_datas.append(row)
        serializer = self.get_serializer(data=input_datas, many=True)
        if serializer.is_valid():
            serializer.save()
            return Response_200(serializer.data)
        raise DjadminException(AssetsError.BATCH_UPLOAD_ERROR,serializer.errors)
        

class ApplicationManage(GenericViewSet,CreateModelMixin,UpdateModelMixin,RetrieveModelMixin,ListModelMixin):
    queryset =  Application.objects.all()
    serializer_class = ApplicationSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter,DjangoFilterBackend,SearchFilter)
    search_fields = ['name', 'remark'] 
    ordering_fields = [ 'name','create_time'] 
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        # view
        'list': 'assets:applications:view',
        'retrieve': 'assets:applications:view',
        # delete
        'batch-delete': 'assets:applications:delete',
        # update
        'partial_update': 'assets:applications:update',
        'perform_update': 'assets:applications:update',
        # create
        'create': 'assets:applications:create',
    }
    # 批量删除Application
    @action(detail=False,methods=['delete'],url_path='batch-delete')
    def batchDelete(self,request):
        # 获取ID数组参数
        ids = request.data.get('ids', [])
        # 先查用户角色列表
        Application.objects.filter(id__in=ids).delete()
        return Response_200()


class HostGroupManage(GenericViewSet,CreateModelMixin,DestroyModelMixin,UpdateModelMixin,RetrieveModelMixin,ListModelMixin):
    queryset = HostGroup.objects.all()
    serializer_class = HostGroupSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    search_fields = ['name', 'remark']
    ordering_fields = ['name', 'create_time']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'assets:hostgroups:view',
        'retrieve': 'assets:hostgroups:view',
        'create': 'assets:hostgroups:create',
        'destroy': 'assets:hostgroups:delete',
        'partial_update': 'assets:hostgroups:update',
        'perform_update': 'assets:hostgroups:update',
        'batch-delete': 'assets:hostgroups:delete',
        'tree': 'assets:hostgroups:view',
    }

    def get_queryset(self):
        if self.action in ['list', 'retrieve', 'tree']:
            return HostGroup.objects.select_related('parent').order_by('id')
        return HostGroup.objects.all()

    def _build_tree(self, group_data):
        nodes = {}
        roots = []

        for item in group_data:
            item['children'] = []
            nodes[item['id']] = item

        for item in group_data:
            parent_id = item.get('parent')
            if parent_id:
                parent = nodes.get(parent_id)
                if parent:
                    parent['children'].append(item)
                else:
                    roots.append(item)
            else:
                roots.append(item)

        return roots

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
            })
        return Response_200(data=data)

    @action(detail=False, methods=['get'], url_path='tree')
    def tree(self, request):
        queryset = self.filter_queryset(self.get_queryset())
        serializer = self.get_serializer(queryset, many=True)
        data = serializer.data
        tree_data = self._build_tree(data)
        return Response_200(data=tree_data)

    def destroy(self, request, *args, **kwargs):
        instance = self.get_object()
        deleted_id = instance.id
        self.perform_destroy(instance)
        return Response_200(data={"id": deleted_id})

    @action(detail=False, methods=['delete'], url_path='batch-delete')
    def batchDelete(self, request):
        ids = request.data.get('ids', [])
        if not ids:
            return Response_200(data=[])
        deleted_count, _ = HostGroup.objects.filter(id__in=ids).delete()
        return Response_200(data={"deleted_count": deleted_count})


class HostManage(GenericViewSet,CreateModelMixin,DestroyModelMixin,UpdateModelMixin,RetrieveModelMixin,ListModelMixin):
    queryset = Host.objects.all()
    serializer_class = HostSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    search_fields = ['instance_name', 'name', 'ip', 'remark']
    ordering_fields = ['name', 'create_time']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'assets:hosts:view',
        'retrieve': 'assets:hosts:view',
        'create': 'assets:hosts:create',
        'destroy': 'assets:hosts:delete',
        'partial_update': 'assets:hosts:update',
        'perform_update': 'assets:hosts:update',
        'batch-delete': 'assets:hosts:delete',
    }

    def get_queryset(self):
        queryset = Host.objects.select_related('group').prefetch_related(
            Prefetch('hostcredential_set', queryset=HostCredential.objects.select_related('credential').filter(is_default=True)),
            'hardware',
            'system',
            'disks',
        ).order_by('-id')
        group_id = self.request.query_params.get('group_id')
        if group_id not in [None, '', '0', 0]:
            queryset = queryset.filter(group_id=group_id)
        return queryset

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
            })
        return Response_200(data=data)

    def destroy(self, request, *args, **kwargs):
        instance = self.get_object()
        deleted_id = instance.id
        self.perform_destroy(instance)
        return Response_200(data={"id": deleted_id})

    @action(detail=False, methods=['delete'], url_path='batch-delete')
    def batchDelete(self, request):
        ids = request.data.get('ids', [])
        if not ids:
            return Response_200(data=[])
        deleted_count, _ = Host.objects.filter(id__in=ids).delete()
        return Response_200(data={"deleted_count": deleted_count})

    