from django.db import models


class LoginAuditLog(models.Model):
    class Status(models.TextChoices):
        SUCCESS = 'success', '登录成功'
        FAILED = 'failed', '登录失败'

    username = models.CharField(max_length=150, blank=True, default='')
    user_id = models.IntegerField(null=True, blank=True)
    status = models.CharField(max_length=16, choices=Status.choices, default=Status.SUCCESS)
    client_ip = models.CharField(max_length=64, blank=True, default='')
    user_agent = models.CharField(max_length=255, blank=True, default='')
    message = models.CharField(max_length=255, blank=True, default='')
    login_time = models.DateTimeField(auto_now_add=True)

    class Meta:
        db_table = 'audit_login_log'
        ordering = ['-login_time']

    def __str__(self):
        return f"{self.username} {self.status}"


class OperationAuditLog(models.Model):
    username = models.CharField(max_length=150, blank=True, default='')
    user_id = models.IntegerField(null=True, blank=True)
    method = models.CharField(max_length=16, blank=True, default='')
    path = models.CharField(max_length=255, blank=True, default='')
    route_name = models.CharField(max_length=255, blank=True, default='')
    client_ip = models.CharField(max_length=64, blank=True, default='')
    user_agent = models.CharField(max_length=255, blank=True, default='')
    request_data = models.TextField(blank=True, default='')
    response_data = models.TextField(blank=True, default='')
    status_code = models.IntegerField(default=200)
    duration_ms = models.IntegerField(null=True, blank=True)
    message = models.CharField(max_length=255, blank=True, default='')
    created_at = models.DateTimeField(auto_now_add=True)

    class Meta:
        db_table = 'audit_operation_log'
        ordering = ['-created_at']

    def __str__(self):
        return f"{self.username} {self.method} {self.path}"
