from django.urls import path,include
from .views import currentUserRoleListView
urlpatterns = [
    path('getCurrentUserRoleList',currentUserRoleListView.as_view(),name='getCurrentUserRoleListView')
]