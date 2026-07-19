from .views_playbook import PlaybookTemplateManage
from .views_shell_script import ShellScriptTemplateManage
from .views_task import AutomationTaskManage
from .views_inventory import AutomationInventoryManage
from .views_job_target import (
    AutomationExecutionJobManage,
)
from .views_workflow import (
    AutomationWorkflowTemplateManage,
    AutomationWorkflowRunManage,
)

__all__ = [
    'PlaybookTemplateManage',
    'ShellScriptTemplateManage',
    'AutomationTaskManage',
    'AutomationInventoryManage',
    'AutomationExecutionJobManage',
    'AutomationWorkflowTemplateManage',
    'AutomationWorkflowRunManage',
]
