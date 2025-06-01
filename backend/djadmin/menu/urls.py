from django.urls import path,include
from .views import *
urlpatterns = [
    path('menus/menuTree/',MenuTree.as_view(),name='menuTree'),
    path('menus/GetMenuListByRoleId/',GetMenuListByRoleId.as_view(),name='GetMenuListByRoleId'),
    path('menus/GrantMenu/',GrantMenu.as_view(),name='GrantMenu'),
    # 新建post
    path('menus/CreateOrUpdateMenuView/',CreateOrUpdateMenuView.as_view(),name='CreateOrUpdateMenuView'),
    # update(put,patch)
    path('menus/CreateOrUpdateMenuView/<int:id>/',CreateOrUpdateMenuView.as_view(),name='CreateOrUpdateMenuView'),
    path('menus/GetMenuById/',GetMenuById.as_view(),name='GetMenuById'),
    # 删除menu
    path('menus/DeleteMenuById/',DeleteMenuById.as_view(),name='DeleteMenuById'),

]