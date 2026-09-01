from django.urls import path, include
from rest_framework.routers import DefaultRouter

from .views_playbook import PlaybookTemplateManage
from .views_task import AutomationTaskManage
from .views_inventory import AutomationInventoryManage
from .views_job_target import AutomationExecutionJobManage

router = DefaultRouter()
router.register(r'playbooks', PlaybookTemplateManage, basename='playbooks')
router.register(r'tasks', AutomationTaskManage, basename='automation-tasks')
router.register(r'inventories', AutomationInventoryManage, basename='automation-inventories')
router.register(r'jobs', AutomationExecutionJobManage, basename='automation-jobs')

urlpatterns = [
    path('', include(router.urls)),
]
