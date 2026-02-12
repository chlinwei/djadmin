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

    password = models.CharField(max_length=128, blank=True, null=True)
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
    name = models.CharField(max_length=128,null=False,default='')
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

    def __str__(self):
        return f"{self.name} ({self.ip})"
    


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

    def __str__(self):
        return f"Hardware of {self.host.name}"
    

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


    def __str__(self):
        return f"System of {self.host.name}"


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
        return f"{self.device} ({self.host.name})"





