from django.urls import path, include
from rest_framework.routers import DefaultRouter
from scheduler import views

router = DefaultRouter()
router.register(r'tasks', views.ScheduledTaskViewSet, basename='scheduled-task')
router.register(r'task-logs', views.ScheduledTaskLogViewSet, basename='scheduled-task-log')

urlpatterns = [
    path('', include(router.urls)),
]