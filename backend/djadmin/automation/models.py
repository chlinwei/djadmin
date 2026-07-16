import uuid

from django.db import models

from djadmin.basemodel import BaseModel


class PlaybookTemplate(BaseModel):
    name = models.CharField(max_length=128, unique=True)
    description = models.CharField(max_length=255, blank=True, default='')
    content = models.TextField(help_text='Playbook YAML content')

    class Meta:
        db_table = 'automation_playbook_template'
        ordering = ['-id']

    def __str__(self):
        return self.name


class AutomationTask(BaseModel):
    class BecomeMethod(models.TextChoices):
        SUDO = 'sudo', 'sudo'
        SU = 'su', 'su'
    
    name = models.CharField(max_length=128, unique=True)
    code = models.CharField(max_length=128, unique=True)
    template = models.ForeignKey(PlaybookTemplate, on_delete=models.PROTECT, related_name='tasks')
    inventory = models.ForeignKey('AutomationInventory', on_delete=models.SET_NULL, null=True, blank=True, related_name='tasks')
    selected_host_ids = models.JSONField(default=list, blank=True)
    selected_group_ids = models.JSONField(default=list, blank=True)
    env_vars = models.JSONField(default=dict, blank=True)
    default_limit = models.CharField(max_length=255, blank=True, default='')
    enabled = models.BooleanField(default=True)
    execution_timeout_seconds = models.PositiveIntegerField(
        default=3600,
        help_text='任务执行超时时间（秒），最大4小时(14400秒)',
        validators=[lambda x: x <= 14400 or (_ for _ in ()).throw(ValueError('Timeout cannot exceed 4 hours (14400 seconds)'))],
    )
    
    # 权限提升配置
    become_enabled = models.BooleanField(default=False, help_text='是否启用权限提升')
    become_method = models.CharField(max_length=16, choices=BecomeMethod.choices, default=BecomeMethod.SUDO, help_text='权限提升方式')
    become_user = models.CharField(max_length=100, default='root', help_text='目标用户')

    class Meta:
        db_table = 'automation_task'
        ordering = ['-id']

    def __str__(self):
        return f'{self.name} ({self.code})'


class AutomationInventory(BaseModel):
    class SyncStatus(models.TextChoices):
        NEVER = 'never', 'Never'
        SUCCESS = 'success', 'Success'
        FAILED = 'failed', 'Failed'

    name = models.CharField(max_length=128, unique=True)
    selected_host_ids = models.JSONField(default=list, blank=True)
    selected_group_ids = models.JSONField(default=list, blank=True)
    enabled = models.BooleanField(default=True)
    update_on_launch = models.BooleanField(default=False)
    update_cache_timeout = models.PositiveIntegerField(default=300)
    last_sync_time = models.DateTimeField(null=True, blank=True)
    last_sync_status = models.CharField(max_length=16, choices=SyncStatus.choices, default=SyncStatus.NEVER)
    last_sync_message = models.TextField(blank=True, default='')
    last_sync_host_count = models.PositiveIntegerField(default=0)

    class Meta:
        db_table = 'automation_inventory'
        ordering = ['-id']

    def __str__(self):
        return self.name


class AnsibleExecutionJob(BaseModel):
    class Status(models.TextChoices):
        PENDING = 'pending', 'Pending'
        RUNNING = 'running', 'Running'
        SUCCESS = 'success', 'Success'
        FAILED = 'failed', 'Failed'
        CANCELLED = 'cancelled', 'Cancelled'

    class TriggerType(models.TextChoices):
        MANUAL = 'manual', 'Manual'
        SCHEDULE = 'schedule', 'Schedule'

    job_id = models.CharField(max_length=36, unique=True, default=uuid.uuid4)
    task = models.ForeignKey(AutomationTask, on_delete=models.SET_NULL, null=True, blank=True, related_name='jobs')
    status = models.CharField(max_length=16, choices=Status.choices, default=Status.PENDING)
    trigger_type = models.CharField(max_length=16, choices=TriggerType.choices, default=TriggerType.MANUAL)
    inventory_snapshot = models.JSONField(default=dict, blank=True)
    task_name_snapshot = models.CharField(max_length=128, blank=True, default='')
    template_name_snapshot = models.CharField(max_length=128, blank=True, default='')
    template_content_snapshot = models.TextField(blank=True, default='')
    extra_vars = models.JSONField(default=dict, blank=True)
    limit = models.CharField(max_length=255, blank=True, default='')
    result_summary = models.JSONField(default=dict, blank=True)
    job_output = models.TextField(blank=True, default='')
    
    # 权限提升配置快照
    become_enabled_snapshot = models.BooleanField(default=False)
    become_method_snapshot = models.CharField(max_length=16, default='sudo')
    become_user_snapshot = models.CharField(max_length=100, default='root')

    requested_user_id = models.IntegerField(null=True, blank=True)
    requested_username = models.CharField(max_length=100, blank=True, default='')

    start_time = models.DateTimeField(null=True, blank=True)
    end_time = models.DateTimeField(null=True, blank=True)
    duration_seconds = models.FloatField(null=True, blank=True)

    class Meta:
        db_table = 'automation_ansible_execution_job'
        ordering = ['-id']

    def __str__(self):
        return f'{self.job_id} [{self.status}]'


class AutomationWorkflowTemplate(BaseModel):
    name = models.CharField(max_length=128, unique=True)
    description = models.CharField(max_length=255, blank=True, default='')
    enabled = models.BooleanField(default=True)
    default_inventory = models.ForeignKey(
        'AutomationInventory',
        on_delete=models.SET_NULL,
        null=True,
        blank=True,
        related_name='workflows',
    )
    default_limit = models.CharField(max_length=255, blank=True, default='')
    entry_node_key = models.CharField(max_length=128)
    nodes = models.JSONField(default=list, blank=True)
    edges = models.JSONField(default=list, blank=True)
    default_extra_vars = models.JSONField(default=dict, blank=True)

    class Meta:
        db_table = 'automation_workflow_template'
        ordering = ['-id']

    def __str__(self):
        return self.name


class AutomationWorkflowRun(BaseModel):
    class Status(models.TextChoices):
        PENDING = 'pending', 'Pending'
        RUNNING = 'running', 'Running'
        SUCCESS = 'success', 'Success'
        FAILED = 'failed', 'Failed'

    class TriggerType(models.TextChoices):
        MANUAL = 'manual', 'Manual'
        SCHEDULE = 'schedule', 'Schedule'

    workflow = models.ForeignKey(AutomationWorkflowTemplate, on_delete=models.SET_NULL, null=True, blank=True, related_name='runs')
    status = models.CharField(max_length=24, choices=Status.choices, default=Status.PENDING)
    trigger_type = models.CharField(max_length=16, choices=TriggerType.choices, default=TriggerType.MANUAL)
    workflow_name_snapshot = models.CharField(max_length=128, blank=True, default='')
    workflow_code_snapshot = models.CharField(max_length=128, blank=True, default='')
    planned_node_keys = models.JSONField(default=list, blank=True)
    node_results = models.JSONField(default=list, blank=True)
    extra_vars = models.JSONField(default=dict, blank=True)
    result_summary = models.JSONField(default=dict, blank=True)

    requested_user_id = models.IntegerField(null=True, blank=True)
    requested_username = models.CharField(max_length=100, blank=True, default='')

    start_time = models.DateTimeField(null=True, blank=True)
    end_time = models.DateTimeField(null=True, blank=True)
    duration_seconds = models.FloatField(null=True, blank=True)

    class Meta:
        db_table = 'automation_workflow_run'
        ordering = ['-id']

    def __str__(self):
        return f'{self.workflow_name_snapshot or self.workflow_id} [{self.status}]'
