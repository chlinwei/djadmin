from django.db import models
from djadmin.basemodel import BaseModel

# Create your models here.


from django.utils import timezone

class Credential(BaseModel):

    class AuthType(models.IntegerChoices):
        PASSWORD = 1, "Password"
        SSH_KEY = 2, "SSH Key"

    name = models.CharField(max_length=200, blank=True, null=True)
    username = models.CharField(max_length=128, null=False, default="root")

    password = models.CharField(max_length=512, blank=True, null=True)
    private_key = models.TextField(blank=True, null=True)
    port = models.PositiveIntegerField(default=22)

    auth_type = models.IntegerField(
        choices=AuthType.choices,
        default=AuthType.PASSWORD
    )





class Application(BaseModel):
    name = models.CharField(default='', max_length=128, null=True, blank=True, verbose_name='应用名称',unique=True)
    version = models.CharField(default='', max_length=128, null=True, blank=True, verbose_name='应用')
    class Meta:
        ordering = ['-id']
        db_table = 'assets_application'



class HostGroup(BaseModel):
    name = models.CharField(max_length=128, unique=True)

    # 可选：支持分组嵌套（以后用）
    parent = models.ForeignKey(
        "self",
        on_delete=models.SET_NULL,
        null=True,
        blank=True,
        related_name="children"
    )

    def __str__(self):
        return self.name


class CloudAccount(BaseModel):

    CLOUD_TYPE_CHOICES = (
        ("vsphere", "vSphere"),
        ("aliyun", "阿里云"),
        ("aws", "AWS"),
        ("manual", "手动"),
    )

    name = models.CharField(max_length=128)

    cloud_type = models.CharField(max_length=32, choices=CLOUD_TYPE_CHOICES)

    # 通用字段
    endpoint = models.CharField(max_length=255, blank=True, null=True)

    # vSphere
    username = models.CharField(max_length=128, blank=True, null=True)
    password = models.CharField(max_length=128, blank=True, null=True)

    # 阿里云
    access_key = models.CharField(max_length=128, blank=True, null=True)
    secret_key = models.CharField(max_length=128, blank=True, null=True)

    
class Host(BaseModel):
    instance_name = models.CharField(max_length=128, blank=True, null=True)
    ip = models.GenericIPAddressField(null=True)
    instance_id = models.CharField(max_length=128, blank=True, null=True)

    cloud_account = models.ForeignKey(
        "CloudAccount",
        on_delete=models.SET_NULL,
        null=True,
        blank=True
    )

    group = models.ForeignKey(
        "HostGroup",
        on_delete=models.SET_NULL,
        null=True,
        blank=True
    )
    status = models.CharField(max_length=32, default="running")
    is_deleted_in_cloud = models.BooleanField(default=False)
    port = models.PositiveIntegerField(default=22)

    # 采集状态（用于在列表中突出无法连接的主机）
    class CollectStatus(models.TextChoices):
        UNKNOWN = "unknown", "未采集"
        SUCCESS = "success", "成功"
        FAILED = "failed", "失败"

    collect_status = models.CharField(
        max_length=16, default=CollectStatus.UNKNOWN, verbose_name="采集状态"
    )
    collect_message = models.TextField(blank=True, default="", verbose_name="采集失败原因")
    collect_time = models.DateTimeField(null=True, blank=True, verbose_name="最后采集时间")

    def __str__(self):
        display_name = self.instance_name or f"Host-{self.id}"
        return f"{display_name} ({self.ip})"
    


from django.db import models
from django.db.models import Q

class HostCredential(BaseModel):
    host = models.ForeignKey("Host", on_delete=models.CASCADE)
    credential = models.ForeignKey("Credential", on_delete=models.CASCADE)
    is_default = models.BooleanField(default=False)

    class Meta:
        unique_together = ("host", "credential")

        constraints = [
            models.UniqueConstraint(
                fields=["host"],
                condition=Q(is_default=True),
                name="unique_default_credential_per_host"
            )
        ]



class HostHardware(BaseModel):
    host = models.OneToOneField(
        "Host",
        on_delete=models.CASCADE,
        related_name="hardware"
    )

    cpu_cores = models.IntegerField(null=True, blank=True)
    cpu_model = models.CharField(max_length=255, blank=True, null=True)

    memory_gb = models.FloatField(null=True, blank=True)

    disk_total_gb = models.FloatField(null=True, blank=True)

    architecture = models.CharField(max_length=64, blank=True, null=True)
    collected_at = models.DateTimeField(null=True, blank=True, verbose_name='最后采集时间')

    def __str__(self):
        host_label = self.host.instance_name or f"Host-{self.host_id}"
        return f"Hardware of {host_label}"
    

class HostSystem(BaseModel):
    host = models.OneToOneField(
        "Host",
        on_delete=models.CASCADE,
        related_name="system"
    )

    os_type = models.CharField(max_length=64, blank=True, null=True)
    os_version = models.CharField(max_length=128, blank=True, null=True)

    kernel_version = models.CharField(max_length=128, blank=True, null=True)

    hostname = models.CharField(max_length=128, blank=True, null=True)

    agent_version = models.CharField(max_length=64, blank=True, null=True)
    collector_source = models.CharField(max_length=32, blank=True, null=True)
    collected_at = models.DateTimeField(null=True, blank=True, verbose_name='最后采集时间')


    def __str__(self):
        host_label = self.host.instance_name or f"Host-{self.host_id}"
        return f"System of {host_label}"


class AgentJob(BaseModel):
    class JobStatus(models.TextChoices):
        QUEUED = 'queued', 'Queued'
        RUNNING = 'running', 'Running'
        SUCCESS = 'success', 'Success'
        FAILED = 'failed', 'Failed'
        CANCELED = 'canceled', 'Canceled'
        TIMEOUT = 'timeout', 'Timeout'

    job_id = models.CharField(max_length=128, unique=True)
    client_request_id = models.CharField(max_length=128, null=True, blank=True, unique=True)
    agent_id = models.CharField(max_length=128)
    host = models.ForeignKey('Host', on_delete=models.SET_NULL, null=True, blank=True, related_name='agent_jobs')
    job_type = models.CharField(max_length=32)
    action = models.CharField(max_length=64)
    params = models.JSONField(default=dict, blank=True)
    timeout_seconds = models.PositiveIntegerField(default=30)
    status = models.CharField(max_length=16, choices=JobStatus.choices, default=JobStatus.QUEUED)
    picked_at = models.DateTimeField(null=True, blank=True)
    finished_at = models.DateTimeField(null=True, blank=True)
    result_data = models.JSONField(default=dict, blank=True)
    error_message = models.TextField(blank=True, default='')

    class Meta:
        db_table = 'assets_agent_job'
        indexes = [
            models.Index(fields=['agent_id', 'status']),
            models.Index(fields=['status', 'create_time']),
        ]

    def __str__(self):
        return f"{self.job_id} ({self.status})"


class AgentJobEvent(BaseModel):
    tag = models.CharField(max_length=255)
    job_id = models.CharField(max_length=128, blank=True, default='')
    agent_id = models.CharField(max_length=128, blank=True, default='')
    event_type = models.CharField(max_length=64, blank=True, default='')
    payload = models.JSONField(default=dict, blank=True)

    class Meta:
        db_table = 'assets_agent_job_event'
        indexes = [
            models.Index(fields=['job_id', 'create_time']),
            models.Index(fields=['agent_id', 'create_time']),
            models.Index(fields=['tag', 'create_time']),
        ]

    def __str__(self):
        return f"{self.tag} ({self.job_id})"


class HostDisk(models.Model):
    host = models.ForeignKey(
        "Host",
        on_delete=models.CASCADE,
        related_name="disks"
    )

    device = models.CharField(max_length=64)
    mount_point = models.CharField(max_length=128, blank=True, null=True)

    size_gb = models.FloatField(null=True, blank=True)
    used_gb = models.FloatField(null=True, blank=True)

    filesystem = models.CharField(max_length=64, blank=True, null=True)

    def __str__(self):
        host_label = self.host.instance_name or f"Host-{self.host_id}"
        return f"{self.device} ({host_label})"


class WebSSHSessionLog(models.Model):
    class Status(models.TextChoices):
        CONNECTED = 'connected', '已连接'
        CLOSED = 'closed', '已关闭'
        FAILED = 'failed', '连接失败'

    host = models.ForeignKey('Host', on_delete=models.CASCADE, related_name='webssh_sessions')
    user_id = models.IntegerField(null=True, blank=True)
    username = models.CharField(max_length=100, blank=True, default='')
    client_ip = models.CharField(max_length=64, blank=True, default='')
    user_agent = models.CharField(max_length=255, blank=True, default='')

    status = models.CharField(max_length=16, choices=Status.choices, default=Status.CONNECTED)
    start_time = models.DateTimeField(auto_now_add=True)
    end_time = models.DateTimeField(null=True, blank=True)
    duration_seconds = models.IntegerField(null=True, blank=True)
    close_code = models.IntegerField(null=True, blank=True)
    error_message = models.TextField(blank=True, default='')

    input_bytes = models.IntegerField(default=0)
    command_count = models.IntegerField(default=0)
    input_content = models.TextField(blank=True, default='')
    output_content = models.TextField(blank=True, default='')
    recorded_content_bytes = models.IntegerField(default=0)
    is_content_truncated = models.BooleanField(default=False)

    class Meta:
        db_table = 'assets_webssh_session_log'
        ordering = ['-start_time']

    def __str__(self):
        return f"{self.id} {self.username} {self.host.id}"




