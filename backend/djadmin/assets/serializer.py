from rest_framework.serializers import ModelSerializer
from datetime import datetime
from .models import *
from rest_framework import serializers



# host user
class CredentialSerializer(ModelSerializer):
    class Meta:
        model = Credential
        fields = '__all__'
    
    # 创建
    def create(self, validated_data):
        validated_data["create_time"] = datetime.now().date()
        data = Credential.objects.create(**validated_data)
        return data
    def validate_private_key(self, value):
        if value and len(value) > 8192:
            raise serializers.ValidationError("私钥长度超过限制")
        return value
# host type
class ApplicationSerializer(ModelSerializer):
    class Meta:
        model = Application
        fields = '__all__'
    
    # 创建
    def create(self, validated_data):
        validated_data["create_time"] = datetime.now().date()
        data = Application.objects.create(**validated_data)
        return data

# host 
class HostSerializer(ModelSerializer):
    class Meta:
        model = Host
        fields = '__all__'
    
    # 创建
    def create(self, validated_data):
        validated_data["create_time"] = datetime.now().date()
        data = Host.objects.create(**validated_data)
        return data


