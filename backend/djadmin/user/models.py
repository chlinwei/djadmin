from django.db import models

# Create your models here.
from role.models import SysRole


class SysUser(models.Model):
    STATUS_CHOICES = [
        (1, '正常'),
        (0, '禁用')
    ]
    id = models.AutoField(primary_key=True)
    username = models.CharField(max_length=100, unique=True, verbose_name="用户名")
    password = models.CharField(max_length=100, verbose_name="密码")
    avatar = models.CharField(max_length=255, null=True, verbose_name="用户头像")
    email = models.CharField(max_length=100, null=True, verbose_name="用户邮箱")
    phonenumber = models.CharField(max_length=11, null=True,blank=True, verbose_name="手机号码")
    login_date = models.DateField(null=True, verbose_name="最后登录时间")
    status = models.SmallIntegerField(choices=STATUS_CHOICES, default=1)
    create_time = models.DateField(null=True,blank=True, verbose_name="创建时间", )
    update_time = models.DateField(null=True,blank=True, verbose_name="更新时间")
    remark = models.CharField(max_length=500, null=True,blank=True,verbose_name="备注")
    class Meta:
        db_table = "sys_user"






# 系统用户角色关联类
class SysUserRole(models.Model):
    id = models.AutoField(primary_key=True)
    role = models.ForeignKey(SysRole, on_delete=models.CASCADE)
    user = models.ForeignKey(SysUser, on_delete=models.CASCADE)
    class Meta:
        db_table = "sys_user_role"
        unique_together = ('user', 'role')
