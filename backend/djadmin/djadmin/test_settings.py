"""
测试专用设置 - 使用独立的 MySQL 测试数据库，不影响生产数据
运行：python manage.py test --settings=djadmin.test_settings
"""
from .settings import *  # type: ignore[wildcard-import]

DATABASES = {
    'default': {
        'ENGINE': 'django.db.backends.mysql',
        'NAME': 'djadmin_test',
        'USER': 'root',
        'PASSWORD': '1qazXSW@',
        'HOST': '10.25.66.150',
        'PORT': '3400',
        'TEST': {
            'NAME': 'djadmin_test',  # 测试数据库名
        },
    }
}
