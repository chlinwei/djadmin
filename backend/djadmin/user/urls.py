from django.urls import path,include
from .views import UserListOrCreateView,TestView
urlpatterns = [
    path('users',UserListOrCreateView.as_view(),name='listOrcreate'),
    path('test',TestView.as_view(),name='test')
]