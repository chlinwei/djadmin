from django.core.validators import MaxValueValidator, MinValueValidator
from django.db import models

from djadmin.basemodel import BaseModel


class InspectionGroup(BaseModel):
    class Scope(models.TextChoices):
        PER_DEPLOYMENT = 'per_deployment', '逻辑服务·每个部署实例'
        SERVICE_ONCE = 'service_once', '逻辑服务·服务单次'
        PER_HOST = 'per_host', '主机组·每台主机'

    name = models.CharField(max_length=128, unique=True, verbose_name='巡检组名称')
    scope = models.CharField(max_length=24, choices=Scope.choices, verbose_name='执行范围')
    description = models.TextField(blank=True, default='', verbose_name='描述')
    enabled = models.BooleanField(default=True, verbose_name='是否启用')

    class Meta:
        db_table = 'inspection_group'
        ordering = ['name', 'id']


class InspectionSeverity(models.TextChoices):
    CRITICAL = 'critical', '严重'
    WARNING = 'warning', '警告'


class InspectionCheck(BaseModel):
    class Executor(models.TextChoices):
        SHELL = 'shell', 'Agent Shell'
        SCHEMA_VALIDATE = 'schema_validate', 'Agent Schema'
        HTTP = 'http', 'Agent HTTP'
        TCP = 'tcp', 'Agent TCP'

    group = models.ForeignKey(InspectionGroup, on_delete=models.CASCADE, related_name='checks')
    name = models.CharField(max_length=128, verbose_name='检查项名称')
    executor = models.CharField(max_length=16, choices=Executor.choices, verbose_name='执行器')
    config = models.JSONField(default=dict, blank=True, verbose_name='检查配置')
    # warning 级检查项失败不会把目标判为失败，只在汇总里计数，用于区分“进程已死”和“磁盘偏高”。
    severity = models.CharField(
        max_length=16,
        choices=InspectionSeverity.choices,
        default=InspectionSeverity.CRITICAL,
        verbose_name='严重级别',
    )
    enabled = models.BooleanField(default=True, verbose_name='是否启用')
    order = models.PositiveIntegerField(default=0, verbose_name='顺序')

    class Meta:
        db_table = 'inspection_check'
        ordering = ['order', 'id']
        constraints = [
            models.UniqueConstraint(fields=['group', 'name'], name='unique_inspection_group_check_name'),
        ]


class InspectionTask(BaseModel):
    class TargetType(models.TextChoices):
        LOGICAL_SERVICE = 'logical_service', '逻辑服务'
        HOST_GROUP = 'host_group', '主机组'

    name = models.CharField(max_length=128, unique=True, verbose_name='任务名称')
    inspection_name = models.CharField(max_length=128, blank=True, default='', verbose_name='巡检名称')
    group = models.ForeignKey(InspectionGroup, on_delete=models.PROTECT, related_name='tasks')
    logical_service = models.ForeignKey(
        'assets.ApplicationService', on_delete=models.PROTECT, related_name='inspection_tasks', null=True, blank=True,
    )
    # 巡检范围只绑定固定主机 ID：分组只是前端勾选入口，事后往组里加主机不会自动进入已有任务。
    selected_host_ids = models.JSONField(default=list, blank=True, verbose_name='勾选主机')
    concurrency = models.PositiveIntegerField(
        default=20,
        validators=[MinValueValidator(1), MaxValueValidator(100)],
        verbose_name='并发数',
    )
    timeout_seconds = models.PositiveIntegerField(
        default=60,
        validators=[MinValueValidator(5), MaxValueValidator(3600)],
        verbose_name='单目标超时',
    )
    # 空表示只允许手动触发；分发器按 next_run_time 到期扫描，不依赖 beat 为每个任务单独建条目。
    cron_expression = models.CharField(max_length=120, blank=True, default='', verbose_name='cron 表达式')
    next_run_time = models.DateTimeField(null=True, blank=True, verbose_name='下次运行时间')
    last_run_time = models.DateTimeField(null=True, blank=True, verbose_name='最近运行时间')
    enabled = models.BooleanField(default=True, verbose_name='是否启用')

    class Meta:
        db_table = 'inspection_task'
        ordering = ['-id']

    @property
    def target_type(self):
        """目标类型由巡检组范围派生，避免两处存储后出现不一致。"""
        return (
            self.TargetType.HOST_GROUP
            if self.group.scope == InspectionGroup.Scope.PER_HOST
            else self.TargetType.LOGICAL_SERVICE
        )


class InspectionExecution(BaseModel):
    class Status(models.TextChoices):
        PENDING = 'pending', '等待中'
        RUNNING = 'running', '执行中'
        SUCCESS = 'success', '成功'
        FAILED = 'failed', '失败'
        CANCELED = 'canceled', '已取消'

    class TriggerType(models.TextChoices):
        MANUAL = 'manual', '手动'
        SCHEDULED = 'scheduled', '定时'

    task = models.ForeignKey(InspectionTask, on_delete=models.SET_NULL, null=True, related_name='executions')
    status = models.CharField(max_length=16, choices=Status.choices, default=Status.PENDING)
    trigger_type = models.CharField(max_length=16, choices=TriggerType.choices, default=TriggerType.MANUAL)
    task_snapshot = models.JSONField(default=dict, blank=True)
    group_snapshot = models.JSONField(default=dict, blank=True)
    service_snapshot = models.JSONField(default=dict, blank=True)
    target_snapshot = models.JSONField(default=list, blank=True)
    summary = models.JSONField(default=dict, blank=True)
    requested_user_id = models.IntegerField(null=True, blank=True)
    requested_username = models.CharField(max_length=100, blank=True, default='')
    start_time = models.DateTimeField(null=True, blank=True)
    end_time = models.DateTimeField(null=True, blank=True)

    class Meta:
        db_table = 'inspection_execution'
        ordering = ['-id']


class InspectionTargetExecution(BaseModel):
    class Status(models.TextChoices):
        PENDING = 'pending', '等待中'
        RUNNING = 'running', '执行中'
        SUCCESS = 'success', '成功'
        FAILED = 'failed', '失败'
        CANCELED = 'canceled', '已取消'

    execution = models.ForeignKey(InspectionExecution, on_delete=models.CASCADE, related_name='targets')
    deployment = models.ForeignKey(
        'assets.ApplicationDeployment', on_delete=models.SET_NULL, null=True, blank=True,
        related_name='inspection_target_executions',
    )
    host = models.ForeignKey(
        'assets.Host', on_delete=models.SET_NULL, null=True, blank=True,
        related_name='inspection_target_executions',
    )
    target_name = models.CharField(max_length=255)
    host_id_snapshot = models.IntegerField(null=True, blank=True)
    host_ip_snapshot = models.CharField(max_length=64, blank=True, default='')
    agent_id_snapshot = models.CharField(max_length=128, blank=True, default='')
    status = models.CharField(max_length=16, choices=Status.choices, default=Status.PENDING)
    passed = models.BooleanField(null=True, blank=True)
    error_message = models.TextField(blank=True, default='')
    raw_result = models.JSONField(default=dict, blank=True)
    start_time = models.DateTimeField(null=True, blank=True)
    end_time = models.DateTimeField(null=True, blank=True)

    class Meta:
        db_table = 'inspection_target_execution'
        ordering = ['id']


class InspectionResult(BaseModel):
    target = models.ForeignKey(InspectionTargetExecution, on_delete=models.CASCADE, related_name='results')
    check_key = models.CharField(max_length=255)
    check_type = models.CharField(max_length=64)
    name = models.CharField(max_length=255)
    status = models.CharField(max_length=16)
    severity = models.CharField(
        max_length=16,
        choices=InspectionSeverity.choices,
        default=InspectionSeverity.CRITICAL,
    )
    expected_value = models.JSONField(null=True, blank=True)
    actual_value = models.JSONField(null=True, blank=True)
    message = models.TextField(blank=True, default='')

    class Meta:
        db_table = 'inspection_result'
        ordering = ['id']