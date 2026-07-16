from .views_playbook import PlaybookTemplateManage
from .views_task import AutomationTaskManage
from .views_inventory import AutomationInventoryManage
from .views_job_target import (
    AnsibleExecutionJobManage,
)
from .views_workflow import (
    AutomationWorkflowTemplateManage,
    AutomationWorkflowRunManage,
)

__all__ = [
    'PlaybookTemplateManage',
    'AutomationTaskManage',
    'AutomationInventoryManage',
    'AnsibleExecutionJobManage',
    'AutomationWorkflowTemplateManage',
    'AutomationWorkflowRunManage',
]
