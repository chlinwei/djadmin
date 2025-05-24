from django_filters import rest_framework as filters
from .models import SysUser
from django.db.models import Q
 
 

from django_filters import rest_framework as filters
from django.db.models import Q
from .models import SysUser

class SysUserFilter(filters.FilterSet):
    keyword = filters.CharFilter(method='multi_field_search')
    order = filters.OrderingFilter(fields=("username",))

    def multi_field_search(self, queryset, name, value):
        return queryset.filter(
            Q(username__icontains=value) |
            Q(email__icontains=value) |
            Q(phonenumber__icontains=value)
        )

    class Meta:
        model = SysUser
        fields = []

