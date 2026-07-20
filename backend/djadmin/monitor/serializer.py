from django.core.exceptions import ValidationError as DjangoValidationError
from rest_framework import serializers
from rest_framework.serializers import ModelSerializer

import hashlib

from .models import MonitorTarget, SoftwarePackage


class MonitorTargetSerializer(ModelSerializer):
    host_name = serializers.SerializerMethodField()
    host_ip = serializers.SerializerMethodField()

    class Meta:
        model = MonitorTarget
        fields = '__all__'

    def get_host_name(self, obj):
        host = getattr(obj, 'host', None)
        if host is None:
            return ''
        return str(getattr(host, 'instance_name', '') or '')

    def get_host_ip(self, obj):
        host = getattr(obj, 'host', None)
        if host is None:
            return ''
        return str(getattr(host, 'ip', '') or '')


class SoftwarePackageSerializer(ModelSerializer):
    download_url = serializers.SerializerMethodField()
    file_name = serializers.SerializerMethodField()
    synced = serializers.SerializerMethodField()
    install_task_name = serializers.SerializerMethodField()
    uninstall_task_name = serializers.SerializerMethodField()

    class Meta:
        model = SoftwarePackage
        fields = '__all__'
        # sha256/size 由上传文件自动计算，不接受客户端传入
        read_only_fields = ('sha256', 'size_bytes', 'create_time', 'update_time')
        # 模型层 file 允许为空是为了支持“未同步”占位记录（由 ensure_defaults 直接创建）；
        # 手动上传/更新接口仍要求必须携带文件，避免通过普通接口创建出空包记录。
        extra_kwargs = {'file': {'required': True}}

    def get_download_url(self, obj):
        file_field = getattr(obj, 'file', None)
        if not file_field:
            return ''
        try:
            return file_field.url
        except ValueError:
            return ''

    def get_file_name(self, obj):
        file_field = getattr(obj, 'file', None)
        if not file_field or not file_field.name:
            return ''
        return file_field.name.rsplit('/', 1)[-1]

    def get_synced(self, obj):
        return bool(getattr(obj, 'file', None) and obj.file.name and obj.sha256)

    def get_install_task_name(self, obj):
        task = getattr(obj, 'install_task', None)
        return str(getattr(task, 'name', '') or '') if task is not None else ''

    def get_uninstall_task_name(self, obj):
        task = getattr(obj, 'uninstall_task', None)
        return str(getattr(task, 'name', '') or '') if task is not None else ''

    def _fill_file_meta(self, validated_data):
        # 上传/替换文件时，流式计算 sha256 并记录字节数，供 agent 下载后完整性校验
        upload_file = validated_data.get('file')
        if upload_file is None:
            return
        hasher = hashlib.sha256()
        for chunk in upload_file.chunks():
            hasher.update(chunk)
        upload_file.seek(0)
        validated_data['sha256'] = hasher.hexdigest()
        validated_data['size_bytes'] = int(getattr(upload_file, 'size', 0) or 0)

    def create(self, validated_data):
        self._fill_file_meta(validated_data)
        return super().create(validated_data)

    def update(self, instance, validated_data):
        self._fill_file_meta(validated_data)
        return super().update(instance, validated_data)
