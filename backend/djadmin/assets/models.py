from django.db import models
from djadmin.basemodel import BaseModel

# Create your models here.


# SSh User
class Credential(BaseModel):
    STATUS = (
        ('0', u'启用'),
        ('1', u'停用'),
    )
    name = models.CharField(default='', max_length=200, null=True, blank=True, verbose_name='SSH 用户')
    password = models.CharField(default='', max_length=128, null=True, blank=True, verbose_name='SSH 密码')
    private_key  = models.TextField(default=None, max_length=8192, null=True,blank=True, verbose_name='SSH 私钥')

    class Meta:
        ordering = ['-id']
        db_table = 'assets_credential'

class Application(BaseModel):
    name = models.CharField(default='', max_length=128, null=True, blank=True, verbose_name='应用名称',unique=True)
    version = models.CharField(default='', max_length=128, null=True, blank=True, verbose_name='应用')
    class Meta:
        ordering = ['-id']
        db_table = 'assets_application'


class Host(BaseModel):
    SSH_STATUS = (
        (0, '下线'),
        (1, '在线'),
    )
    OPERATION_STATUS = (
        (0, '下线'),
        (1, '在线'),
    )
    application = models.ForeignKey(Application,default='', null=True, blank=True, on_delete=models.SET_NULL, verbose_name='主机类别')
    host_credential = models.ForeignKey(Credential, default='', null=True, blank=True, on_delete=models.SET_NULL, verbose_name='SSH用户')
    hostname = models.CharField(default='',blank=True,null=True,max_length=200,verbose_name='主机名')
    ssh_ip = models.CharField(default='', max_length=128, null=True, blank=True, verbose_name='SSH IP地址')
    ssh_port = models.PositiveIntegerField(default=22, null=True, blank=True, verbose_name='SSH 端口')
    cpu = models.CharField(default='', max_length=64, null=True, blank=True, verbose_name='CPU')
    memory = models.CharField(default='', max_length=64, null=True, blank=True, verbose_name='内存')
    disk = models.CharField(default='', max_length=64, null=True, blank=True, verbose_name='磁盘大小')
    ssh_status = models.PositiveBigIntegerField(default=0,choices=SSH_STATUS, verbose_name='ssh状态')
    operation_status = models.PositiveBigIntegerField(default=0,choices=OPERATION_STATUS,verbose_name='运营状态')
    
    class Meta:
        ordering = ['-id']
        db_table = 'assets_host'
    def __str__(self):
        return self.hostname

