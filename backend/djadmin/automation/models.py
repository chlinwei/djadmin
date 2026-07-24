import uuid

from django.db import models
from django.core.validators import MaxValueValidator

from djadmin.basemodel import BaseModel


def validate_timeout(value):
    """验证执行超时时间不超过4小时（14400秒）"""
    if value > 14400:
        raise ValueError('Timeout cannot exceed 4 hours (14400 seconds)')


class TemplateCategory(models.TextChoices):
    """模板分类：区分通用运维模板与监控软件包安装/卸载专用模板，避免二者在“模板”列表页混在一起。"""
    GENERAL = 'general', '通用'
    SOFTWARE_PACKAGE = 'software_package', '软件包安装/卸载专用'


class PlaybookTemplate(BaseModel):
    name = models.CharField(max_length=128, unique=True)
    description = models.CharField(max_length=255, blank=True, default='')
    content = models.TextField(help_text='Playbook YAML content')
    category = models.CharField(
        max_length=32, choices=TemplateCategory.choices, default=TemplateCategory.GENERAL,
        help_text='模板分类：通用 / 软件包安装卸载专用，用于列表页默认分组展示',
    )

    class Meta:
        db_table = 'automation_playbook_template'
        ordering = ['-id']

    def __str__(self):
        return self.name


class ShellScriptTemplate(BaseModel):
    name = models.CharField(max_length=128, unique=True)
    description = models.CharField(max_length=255, blank=True, default='')
    content = models.TextField(help_text='Shell script content')
    category = models.CharField(
        max_length=32, choices=TemplateCategory.choices, default=TemplateCategory.GENERAL,
        help_text='模板分类：通用 / 软件包安装卸载专用，用于列表页默认分组展示',
    )

    class Meta:
        db_table = 'automation_shell_script_template'
        ordering = ['-id']

    def __str__(self):
        return self.name


class AutomationTask(BaseModel):
    name = models.CharField(max_length=128, unique=True)
    # 支持 Playbook 或 ShellScript 执行方式
    playbook_template = models.ForeignKey(PlaybookTemplate, on_delete=models.PROTECT, related_name='tasks', null=True, blank=True)
    shell_script_template = models.ForeignKey(ShellScriptTemplate, on_delete=models.PROTECT, related_name='tasks', null=True, blank=True)
    inventory = models.ForeignKey('AutomationInventory', on_delete=models.SET_NULL, null=True, blank=True, related_name='tasks')
    selected_host_ids = models.JSONField(default=list, blank=True)
    selected_group_ids = models.JSONField(default=list, blank=True)
    shell_parameters = models.TextField(blank=True, default='')
    env_vars = models.JSONField(default=dict, blank=True)
    default_limit = models.CharField(max_length=255, blank=True, default='')
    enabled = models.BooleanField(default=True)
    execution_timeout_seconds = models.PositiveIntegerField(
        default=600,
        help_text='任务执行超时时间（秒），最大4小时(14400秒)',
        validators=[validate_timeout],
    )

    # 执行身份配置：dj-agent 进程本身以 root 运行，任务实际执行时通过 setuid/setgid 降权到这里指定的
    # 系统用户/组（见 dj_agent/internal/executor/automation.go resolveRunAsCredential），不再使用
    # ansible become/sudo 机制。run_as_user 必填（不允许空值静默以 root 执行，避免权限提升场景被遗漏配置）；
    # run_as_group 留空时使用 run_as_user 的主组。
    run_as_user = models.CharField(max_length=100, help_text='任务执行时降权切换到的系统用户（必填，不允许留空以 root 身份静默执行）')
    run_as_group = models.CharField(max_length=100, blank=True, default='', help_text='任务执行时切换到的系统组，留空则使用 run_as_user 的主组')
    # 任务执行时的工作目录：默认为 /tmp，可按任务覆盖。
    work_directory = models.CharField(max_length=255, blank=True, default='/tmp', help_text='任务执行时的工作目录，默认为 /tmp')

    class Meta:
        db_table = 'automation_task'
        ordering = ['-id']

    def __str__(self):
        return self.name


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


class AutomationExecutionJob(BaseModel):
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
    shell_parameters = models.TextField(blank=True, default='')
    shell_env_vars = models.JSONField(default=dict, blank=True)
    extra_vars = models.JSONField(default=dict, blank=True)
    limit = models.CharField(max_length=255, blank=True, default='')
    result_summary = models.JSONField(default=dict, blank=True)

    # 执行身份/工作目录快照：任务创建 job 时从 AutomationTask.run_as_user/run_as_group/work_directory
    # 复制而来，保证即使之后任务配置被修改，历史执行记录仍反映当次真实使用的身份与目录。
    run_as_user_snapshot = models.CharField(max_length=100, blank=True, default='')
    run_as_group_snapshot = models.CharField(max_length=100, blank=True, default='')
    work_directory_snapshot = models.CharField(max_length=255, blank=True, default='/tmp')

    requested_user_id = models.IntegerField(null=True, blank=True)
    requested_username = models.CharField(max_length=100, blank=True, default='')

    start_time = models.DateTimeField(null=True, blank=True)
    end_time = models.DateTimeField(null=True, blank=True)
    duration_seconds = models.FloatField(null=True, blank=True)

    class Meta:
        db_table = 'automation_execution_job'
        ordering = ['-id']

    def __str__(self):
        return f'{self.job_id} [{self.status}]'


class AutomationExecutionTargetLog(BaseModel):
    class Status(models.TextChoices):
        PENDING = 'pending', 'Pending'
        RUNNING = 'running', 'Running'
        SUCCESS = 'success', 'Success'
        FAILED = 'failed', 'Failed'
        CANCELLED = 'cancelled', 'Cancelled'

    job = models.ForeignKey(AutomationExecutionJob, on_delete=models.CASCADE, related_name='target_logs')
    host = models.ForeignKey('assets.Host', on_delete=models.SET_NULL, null=True, blank=True, related_name='automation_host_logs')
    host_id_snapshot = models.IntegerField(null=True, blank=True)
    host_name_snapshot = models.CharField(max_length=128, blank=True, default='')
    host_ip_snapshot = models.CharField(max_length=64, blank=True, default='')
    agent_job_id = models.CharField(max_length=64, blank=True, default='')
    status = models.CharField(max_length=16, choices=Status.choices, default=Status.FAILED)
    exit_code = models.IntegerField(null=True, blank=True)
    stdout = models.TextField(blank=True, default='')
    stderr = models.TextField(blank=True, default='')
    error_message = models.TextField(blank=True, default='')
    result_data = models.JSONField(default=dict, blank=True)

    class Meta:
        db_table = 'automation_execution_host_log'
        ordering = ['id']

    def __str__(self):
        host_label = self.host_name_snapshot or (f'Host-{self.host_id_snapshot}' if self.host_id_snapshot else 'Unknown')
        job_pk = getattr(self, 'job_id', None)
        return f'job={job_pk} host={host_label} status={self.status}'


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
        workflow_id = getattr(self, 'workflow_id', None)
        return f'{self.workflow_name_snapshot or workflow_id} [{self.status}]'
