from django.db import models
from djadmin.basemodel import BaseModel


class SysConfig(BaseModel):
    update_time = models.DateTimeField(auto_now=True, verbose_name='修改时间')

    VALUE_TYPE_CHOICES = [
        ('string', '字符串'),
        ('int', '整数'),
        ('bool', '布尔值'),
        ('json', 'JSON'),
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
        return self.value
