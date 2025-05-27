from django.urls import path,include
from .views import *
urlpatterns = [
    path('roles/getCurrentUserRoleList',currentUserRoleListView.as_view(),name='getCurrentUserRoleListView'),
    path('roles/getRoleList/',RoleListView.as_view(),name='getRoleList'),
]