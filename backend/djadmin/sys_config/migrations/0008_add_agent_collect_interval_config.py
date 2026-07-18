from django.db import migrations


def forwards_add_agent_collect_interval_config(apps, schema_editor):
    SysConfig = apps.get_model('sys_config', 'SysConfig')
    SysConfig.objects.get_or_create(
        key='sys.assets.collect.interval_seconds',
        defaults={
            'value': '40',
            'default_value': '40',
            'value_type': 'int',
            'name': '主机信息采集间隔（秒）',
            'description': 'Agent 主机信息周期上报间隔（秒）',
            'is_readonly': False,
        },
    )


def backwards_remove_agent_collect_interval_config(apps, schema_editor):
    SysConfig = apps.get_model('sys_config', 'SysConfig')
    SysConfig.objects.filter(key='sys.assets.collect.interval_seconds').delete()


class Migration(migrations.Migration):

    dependencies = [
        ('sys_config', '0007_remove_collect_auth_lock_minutes'),
    ]

    operations = [
        migrations.RunPython(
            forwards_add_agent_collect_interval_config,
            backwards_remove_agent_collect_interval_config,
        ),
    ]
