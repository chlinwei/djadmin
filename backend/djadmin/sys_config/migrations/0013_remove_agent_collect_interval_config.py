from django.db import migrations


AGENT_HOST_COLLECT_INTERVAL_SECONDS_KEY = 'sys.assets.collect.interval_seconds'


def forwards_remove_agent_collect_interval_config(apps, schema_editor):
    SysConfig = apps.get_model('sys_config', 'SysConfig')
    SysConfig.objects.filter(key=AGENT_HOST_COLLECT_INTERVAL_SECONDS_KEY).delete()


def backwards_restore_agent_collect_interval_config(apps, schema_editor):
    SysConfig = apps.get_model('sys_config', 'SysConfig')
    SysConfig.objects.get_or_create(
        key=AGENT_HOST_COLLECT_INTERVAL_SECONDS_KEY,
        defaults={
            'value': '40',
            'default_value': '40',
            'value_type': 'int',
            'name': '主机信息采集间隔（秒）',
            'description': 'Agent 主机信息周期上报间隔（秒）',
            'is_readonly': False,
        },
    )


class Migration(migrations.Migration):

    dependencies = [
        ('sys_config', '0012_alter_sysconfig_value_type'),
    ]

    operations = [
        migrations.RunPython(
            forwards_remove_agent_collect_interval_config,
            backwards_restore_agent_collect_interval_config,
        ),
    ]
