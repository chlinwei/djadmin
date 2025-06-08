from django.shortcuts import render
from djadmin.utils import Response_200
from rest_framework.mixins import CreateModelMixin,UpdateModelMixin,RetrieveModelMixin,ListModelMixin
from rest_framework.viewsets import GenericViewSet
from rest_framework.decorators import action
from .models import *
from .serializer import *
from djadmin.utils import CustomPagination
from rest_framework.filters import OrderingFilter,SearchFilter
from django_filters.rest_framework  import DjangoFilterBackend




class Host_credentialManage(GenericViewSet,CreateModelMixin,UpdateModelMixin,RetrieveModelMixin,ListModelMixin):
    queryset =  Credential.objects.all()
    serializer_class = CredentialSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter,DjangoFilterBackend,SearchFilter)
    search_fields = ['name', 'remark'] 
    ordering_fields = [ 'name','create_time'] 
    lookup_field = 'id'
    