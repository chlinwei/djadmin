from django.urls import path, include
from rest_framework.routers import DefaultRouter

from .views import (
    PlaybookTemplateManage,
    ShellScriptTemplateManage,
    AutomationTaskManage,
    AutomationInventoryManage,
    AnsibleExecutionJobManage,
    AutomationWorkflowTemplateManage,
    AutomationWorkflowRunManage,
)

router = DefaultRouter()
router.register(r'playbooks', PlaybookTemplateManage, basename='playbooks')
router.register(r'shell-script-templates', ShellScriptTemplateManage, basename='shell-script-templates')
router.register(r'tasks', AutomationTaskManage, basename='automation-tasks')
router.register(r'inventories', AutomationInventoryManage, basename='automation-inventories')
router.register(r'workflows', AutomationWorkflowTemplateManage, basename='automation-workflows')
router.register(r'workflow-runs', AutomationWorkflowRunManage, basename='automation-workflow-runs')
router.register(r'jobs', AnsibleExecutionJobManage, basename='automation-jobs')

urlpatterns = [
    path('', include(router.urls)),
]
