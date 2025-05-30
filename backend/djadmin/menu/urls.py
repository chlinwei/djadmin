from django.urls import path,include
from .views import *
urlpatterns = [
    path('menus/menuTree/',MenuTree.as_view(),name='menuTree'),
]