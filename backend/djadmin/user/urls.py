from django.urls import path,include
from .views import UserListOrCreateView,TestView,LoginView
urlpatterns = [
    path('users',UserListOrCreateView.as_view(),name='listOrcreate'),
    path('test',TestView.as_view(),name='test'),
    path('login',LoginView.as_view(),name='login')
]