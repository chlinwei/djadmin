from django.contrib.auth.hashers import check_password, make_password
from django.db import models
from djadmin.basemodel import BaseModel

# 前端展示 secret 类型参数时的统一掩码占位符：永远不回显明文/哈希原文，
# 且约定“提交这个占位符 = 不修改原值”，避免编辑表单未改动就把明文覆盖成占位符本身的哈希。
SECRET_MASK_PLACEHOLDER = '******'


class SysConfig(BaseModel):
    update_time = models.DateTimeField(auto_now=True, verbose_name='修改时间')

    VALUE_TYPE_CHOICES = [
        ('string', '字符串'),
        ('int', '整数'),
        ('bool', '布尔值'),
        ('json', 'JSON'),
        ('secret', '密文（哈希存储，不回显明文）'),
    ]

    key = models.CharField(max_length=128, unique=True, verbose_name='参数键')
    value = models.TextField(verbose_name='参数值')
    default_value = models.TextField(blank=True, null=True, verbose_name='默认值')
    value_type = models.CharField(max_length=16, choices=VALUE_TYPE_CHOICES, default='string', verbose_name='值类型')
    name = models.CharField(max_length=128, verbose_name='参数名称')
    description = models.TextField(blank=True, null=True, verbose_name='说明')
    is_readonly = models.BooleanField(default=False, verbose_name='只读')

    class Meta:
        db_table = 'sys_config'
        ordering = ['id']
        verbose_name = '系统参数'

    def __str__(self):
        return f'{self.name}({self.key})'

    def get_typed_value(self):
        """返回根据 value_type 转换后的值"""
        if self.value_type == 'int':
            try:
                return int(self.value)
            except (ValueError, TypeError):
                return self.value
        elif self.value_type == 'bool':
            return self.value.lower() in ('true', '1', 'yes')
        elif self.value_type == 'json':
            import json
            try:
                return json.loads(self.value)
            except (ValueError, TypeError):
                return self.value
        elif self.value_type == 'secret':
            # secret 类型的 value 字段存的是哈希，任何读取路径都不能把它当明文返回。
            return SECRET_MASK_PLACEHOLDER if self.value else ''
        return self.value

    def set_secret_value(self, plaintext):
        """写入 secret 类型参数：哈希落库，永不保存明文。"""
        self.value = make_password(str(plaintext or ''))

    def check_secret_value(self, candidate):
        """校验 secret 类型参数：Django 密码哈希内部已做恒定时间比较，避免时序攻击。"""
        if not self.value or not candidate:
            return False
        return check_password(str(candidate), self.value)
