from rest_framework import serializers
from rest_framework.serializers import ModelSerializer

from .models import MonitorTarget


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
