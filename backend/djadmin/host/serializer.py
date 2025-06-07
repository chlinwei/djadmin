from rest_framework.serializers import ModelSerializer
from datetime import datetime
from .models import *



# host user
class Host_UserSerializer(ModelSerializer):
    class Meta:
        model = Host_User
        fields = '__all__'
    
    # 创建
    def create(self, validated_data):
        validated_data["create_time"] = datetime.now().date()
        data = Host_User.objects.create(**validated_data)
        return data
# host type
class Host_TypeSerializer(ModelSerializer):
    class Meta:
        model = Host_Type
        fields = '__all__'
    
    # 创建
    def create(self, validated_data):
        validated_data["create_time"] = datetime.now().date()
        data = Host_Type.objects.create(**validated_data)
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


