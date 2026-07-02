from datetime import datetime

from rest_framework import serializers
from rest_framework.serializers import ModelSerializer

from .models import PlaybookTemplate, AnsibleExecutionJob, AnsibleExecutionTarget


class PlaybookTemplateSerializer(ModelSerializer):
    class Meta:
        model = PlaybookTemplate
        fields = '__all__'

    def create(self, validated_data):
        validated_data['create_time'] = datetime.now().date()
        return PlaybookTemplate.objects.create(**validated_data)


class AnsibleExecutionTargetSerializer(ModelSerializer):
    class Meta:
        model = AnsibleExecutionTarget
        fields = '__all__'


class AnsibleExecutionJobSerializer(ModelSerializer):
    targets = AnsibleExecutionTargetSerializer(many=True, read_only=True)
    template_name = serializers.SerializerMethodField()

    class Meta:
        model = AnsibleExecutionJob
        fields = '__all__'

    def get_template_name(self, obj):
        return obj.template.name if obj.template_id else ''

    def create(self, validated_data):
        validated_data['create_time'] = datetime.now().date()
        return AnsibleExecutionJob.objects.create(**validated_data)
