from django.db import models
from djadmin.basemodel import BaseModel


class ScheduledTask(BaseModel):
    name = models.CharField(max_length=128, unique=True)
    code = models.CharField(max_length=128, unique=True)
    description = models.CharField(max_length=255, blank=True, null=True)
    menu = models.ForeignKey(
        'menu.SysMenu',
        on_delete=models.SET_NULL,
        blank=True,
        null=True,
        related_name='scheduled_tasks',
        verbose_name='关联菜单'
    )
    enabled = models.BooleanField(default=True)
    is_running = models.BooleanField(default=False, verbose_name='正在运行')
    interval_minutes = models.IntegerField(blank=True, null=True)
    last_run_time = models.DateTimeField(blank=True, null=True)
    next_run_time = models.DateTimeField(blank=True, null=True, verbose_name='下次运行时间')
    last_status = models.CharField(max_length=32, blank=True, null=True)
    last_message = models.TextField(blank=True, null=True)

    def __str__(self):
        return f"{self.name} ({self.code})"


class ScheduledTaskLog(BaseModel):
    task = models.ForeignKey(
        ScheduledTask,
        on_delete=models.CASCADE,
        related_name='logs'
    )
    run_time = models.DateTimeField()
    status = models.CharField(max_length=32)
    message = models.TextField(blank=True, null=True)
    duration_seconds = models.FloatField(blank=True, null=True)
    output = models.TextField(blank=True, null=True, help_text='Task execution output/logs')

    def __str__(self):
        return f"{self.task.name} @ {self.run_time} [{self.status}]"
