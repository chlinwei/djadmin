from django.urls import path, include
from rest_framework.routers import DefaultRouter

from .views import PlaybookTemplateManage, AutomationTaskManage, AutomationInventoryManage, AnsibleExecutionJobManage, AnsibleExecutionTargetManage

router = DefaultRouter()
router.register(r'playbooks', PlaybookTemplateManage, basename='playbooks')
router.register(r'tasks', AutomationTaskManage, basename='automation-tasks')
router.register(r'inventories', AutomationInventoryManage, basename='automation-inventories')
router.register(r'jobs', AnsibleExecutionJobManage, basename='automation-jobs')
router.register(r'targets', AnsibleExecutionTargetManage, basename='automation-targets')

urlpatterns = [
    path('', include(router.urls)),
]
