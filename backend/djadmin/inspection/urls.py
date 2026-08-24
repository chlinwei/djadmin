from django.urls import include, path
from rest_framework.routers import DefaultRouter

from .views import InspectionExecutionViewSet, InspectionGroupViewSet, InspectionTaskViewSet

router = DefaultRouter()
router.register('groups', InspectionGroupViewSet, basename='inspection-groups')
router.register('tasks', InspectionTaskViewSet, basename='inspection-tasks')
router.register('executions', InspectionExecutionViewSet, basename='inspection-executions')

urlpatterns = [path('', include(router.urls))]