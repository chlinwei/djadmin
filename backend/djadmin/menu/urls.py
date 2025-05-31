from django.urls import path,include
from .views import *
urlpatterns = [
    path('menus/menuTree/',MenuTree.as_view(),name='menuTree'),
    path('menus/GetMenuListByRoleId/',GetMenuListByRoleId.as_view(),name='GetMenuListByRoleId'),
    path('menus/GrantMenu/',GrantMenu.as_view(),name='GrantMenu'),
    # 新建和保存menu
    path('menus/SaveOrCreateMenuView/',SaveOrCreateMenuView.as_view(),name='SaveOrCreateMenuView'),
    path('menus/GetMenuById/',GetMenuById.as_view(),name='GetMenuById'),

]