import uuid

from django.db import models

from assets.models import Host
from djadmin.basemodel import BaseModel


class PlaybookTemplate(BaseModel):
    name = models.CharField(max_length=128, unique=True)
    code = models.CharField(max_length=128, unique=True)
    description = models.CharField(max_length=255, blank=True, default='')
    content = models.TextField(help_text='Playbook YAML content')
    default_extra_vars = models.JSONField(default=dict, blank=True)
    enabled = models.BooleanField(default=True)

    class Meta:
        db_table = 'automation_playbook_template'
        ordering = ['-id']

    def __str__(self):
        return f'{self.name} ({self.code})'


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
    template = models.ForeignKey(PlaybookTemplate, on_delete=models.PROTECT, related_name='jobs')
    status = models.CharField(max_length=16, choices=Status.choices, default=Status.PENDING)
    trigger_type = models.CharField(max_length=16, choices=TriggerType.choices, default=TriggerType.MANUAL)
    inventory_snapshot = models.JSONField(default=dict, blank=True)
    extra_vars = models.JSONField(default=dict, blank=True)
    result_summary = models.JSONField(default=dict, blank=True)

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


class AnsibleExecutionTarget(BaseModel):
    class Status(models.TextChoices):
        PENDING = 'pending', 'Pending'
        RUNNING = 'running', 'Running'
        SUCCESS = 'success', 'Success'
        FAILED = 'failed', 'Failed'
        SKIPPED = 'skipped', 'Skipped'
        UNREACHABLE = 'unreachable', 'Unreachable'

    job = models.ForeignKey(AnsibleExecutionJob, on_delete=models.CASCADE, related_name='targets')
    host = models.ForeignKey(Host, on_delete=models.SET_NULL, null=True, blank=True, related_name='ansible_targets')

    host_name = models.CharField(max_length=128, blank=True, default='')
    host_ip = models.CharField(max_length=64, blank=True, default='')

    status = models.CharField(max_length=16, choices=Status.choices, default=Status.PENDING)
    rc = models.IntegerField(null=True, blank=True)

    stdout = models.TextField(blank=True, default='')
    stderr = models.TextField(blank=True, default='')

    start_time = models.DateTimeField(null=True, blank=True)
    end_time = models.DateTimeField(null=True, blank=True)
    duration_seconds = models.FloatField(null=True, blank=True)

    class Meta:
        db_table = 'automation_ansible_execution_target'
        ordering = ['id']

    def __str__(self):
        return f'{self.job.job_id} - {self.host_name or self.host_ip}'
