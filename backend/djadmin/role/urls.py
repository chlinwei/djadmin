from django.urls import path
from .views import *
urlpatterns = [
    path('roles/getCurrentUserRoleList/',currentUserRoleListView.as_view(),name='getCurrentUserRoleListView'),
    path('roles/list/',RoleListView.as_view(),name='getRoleList'),
    path('roles/getUserRoleList/',GetUserRolesByIdView.as_view(),name='getUserRoleList'),

]