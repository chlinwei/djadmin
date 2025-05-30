
from django_filters import rest_framework as filters
from django.db.models import Q
 
 

from django_filters import rest_framework as filters
from django.db.models import Q
from .models import SysRole

class SysRoleFilter(filters.FilterSet):
    keyword = filters.CharFilter(method='multi_field_search')
    order = filters.OrderingFilter(fields=("name",))

    def multi_field_search(self, queryset, name, value):
        return queryset.filter(
            Q(name__icontains=value) |
            Q(code__icontains=value) 
        )

    class Meta:
        model = SysRole
        fields = []
