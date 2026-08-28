from datetime import date

from django.db import migrations

# 原先硬编码在 log_management.RETENTION_TIERS 的三个档位改为数据；
# 日写入量是新引入的容量规划字段，按经验值预置，用户可在页面上调整。
DEFAULT_TIERS = [
    {'code': 'hot', 'name': '热（7 天）', 'daily_size_gb': 20, 'retention_days': 7,
     'rollover_min_index_age': '1d', 'is_default': False},
    {'code': 'std', 'name': '标准（30 天）', 'daily_size_gb': 5, 'retention_days': 30,
     'rollover_min_index_age': '1d', 'is_default': True},
    {'code': 'cold', 'name': '冷（90 天）', 'daily_size_gb': 0.1, 'retention_days': 90,
     'rollover_min_index_age': '1d', 'is_default': False},
]


def seed_tiers(apps, schema_editor):
    LogRetentionTier = apps.get_model('monitor', 'LogRetentionTier')
    today = date.today()
    for tier in DEFAULT_TIERS:
        LogRetentionTier.objects.update_or_create(
            code=tier['code'],
            defaults={**tier, 'enabled': True, 'create_time': today, 'update_time': today},
        )


def drop_tiers(apps, schema_editor):
    LogRetentionTier = apps.get_model('monitor', 'LogRetentionTier')
    LogRetentionTier.objects.filter(code__in=[tier['code'] for tier in DEFAULT_TIERS]).delete()


class Migration(migrations.Migration):

    dependencies = [
        ('monitor', '0061_logretentiontier'),
    ]

    operations = [
        migrations.RunPython(seed_tiers, drop_tiers),
    ]
