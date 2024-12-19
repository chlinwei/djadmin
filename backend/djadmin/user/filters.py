from django_filters import rest_framework as filters
from .models import SysUser
 
 
class SysUserFilter(filters.FilterSet):
 
 
    class Meta:
        model = SysUser
        fields = ['username', 'status']