from django.db import models
from djadmin.basemodel import BaseModel

# Create your models here.


# SSh User
class Host_User(BaseModel):
    STATUS = (
        ('0', u'启用'),
        ('1', u'停用'),
    )
    name = models.CharField(default='', max_length=200, null=True, blank=True, verbose_name='SSH 用户')
    password = models.CharField(default='', max_length=128, null=True, blank=True, verbose_name='SSH 密码')

    class Meta:
        ordering = ['-id']
        db_table = 'assets_host_User'

class Host_Type(BaseModel):
    name = models.CharField(default='', max_length=128, null=True, blank=True, verbose_name='Host类别')
    class Meta:
        ordering = ['-id']
        db_table = 'assets_host_type'


class Host(BaseModel):
    STATUS = (
        ('0', u'下线'),
        ('1', u'在线'),
    )
    host_type = models.ForeignKey(Host_Type,default='', null=True, blank=True, on_delete=models.SET_NULL, verbose_name='主机类别')
    hostname = models.CharField(default='',blank=True,null=True,max_length=200,verbose_name='主机名')
    ssh_user = models.ForeignKey(Host_User, default='', null=True, blank=True, on_delete=models.SET_NULL, verbose_name='SSH用户')
    ssh_ip = models.CharField(default='', max_length=128, null=True, blank=True, verbose_name='SSH IP地址')
    ssh_port = models.PositiveIntegerField(default=22, null=True, blank=True, verbose_name='SSH 端口')
    cpu = models.CharField(default='', max_length=64, null=True, blank=True, verbose_name='CPU')
    memory = models.CharField(default='', max_length=64, null=True, blank=True, verbose_name='内存')
    disk = models.CharField(default='', max_length=64, null=True, blank=True, verbose_name='磁盘大小')
    system_product = models.CharField(default='', max_length=128, null=True, blank=True, verbose_name='服务器类型')
    status = models.CharField(default='1', max_length=2, choices=STATUS, verbose_name='运行状态')
    class Meta:
        ordering = ['-id']
        db_table = 'assets_host'
    def __str__(self):
        return self.hostname
