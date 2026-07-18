from django.db import models
from django.contrib.auth.hashers import check_password, identify_hasher, make_password

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
    login_date = models.DateTimeField(null=True, verbose_name="最后登录时间")
    status = models.SmallIntegerField(choices=STATUS_CHOICES, default=1)
    create_time = models.DateField(null=True,blank=True, verbose_name="创建时间", )
    update_time = models.DateField(null=True,blank=True, verbose_name="更新时间")
    remark = models.CharField(max_length=500, null=True,blank=True,verbose_name="备注")
    timezone = models.CharField(max_length=50, default='UTC', verbose_name="时区")

    def set_password(self, raw_password):
        self.password = make_password(raw_password)

    def _is_hashed_password(self):
        try:
            identify_hasher(self.password)
            return True
        except Exception:
            return False

    def check_password(self, raw_password, auto_upgrade=False):
        if self._is_hashed_password():
            if not auto_upgrade:
                return check_password(raw_password, self.password)

            def setter(new_raw_password):
                self.set_password(new_raw_password)
                if self.pk:
                    self.save(update_fields=['password'])

            return check_password(raw_password, self.password, setter=setter)

        # 兼容历史明文密码：登录成功后自动迁移为哈希
        if raw_password == self.password:
            if auto_upgrade:
                self.set_password(raw_password)
                if self.pk:
                    self.save(update_fields=['password'])
            return True
        return False

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


class ApiToken(models.Model):
    BIND_MODE_CHOICES = [
        ('agent', 'Agent共享'),
        ('api', 'Api绑定'),
    ]

    id = models.AutoField(primary_key=True)
    agent_id = models.CharField(max_length=128, verbose_name="Api绑定标识")
    bind_mode = models.CharField(max_length=32, choices=BIND_MODE_CHOICES, default='api', verbose_name="绑定模式")
    token_hash = models.CharField(max_length=255, verbose_name="Token哈希")
    name = models.CharField(max_length=128, null=True, blank=True, verbose_name="名称")
    is_active = models.BooleanField(default=True, verbose_name="是否启用")
    expires_at = models.DateTimeField(null=True, blank=True, verbose_name="过期时间")
    last_used_at = models.DateTimeField(null=True, blank=True, verbose_name="最后使用时间")
    created_by = models.ForeignKey(
        SysUser,
        null=True,
        blank=True,
        on_delete=models.SET_NULL,
        related_name='created_api_tokens',
        verbose_name="创建人",
    )
    remark = models.CharField(max_length=500, null=True, blank=True, verbose_name="备注")
    create_time = models.DateTimeField(auto_now_add=True, verbose_name="创建时间")
    update_time = models.DateTimeField(auto_now=True, verbose_name="更新时间")

    class Meta:
        db_table = "sys_agent_token"
