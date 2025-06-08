from django.db import models
from django.utils import timezone
from datetime import datetime

class BaseModel(models.Model):
    '''
       基础表(抽象类)
    '''
    create_time = models.DateField( verbose_name='创建时间', default=datetime.now().date())
    update_time = models.DateField(auto_now=True, verbose_name='修改时间')
    remark = models.TextField(default='', null=True, blank=True, verbose_name='备注')
    class Meta:
        abstract = True