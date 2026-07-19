from django.db import migrations


HOST_MANAGE_REFRESH_INTERVAL_SECONDS_KEY = 'sys.assets.host.manage.refresh_interval_seconds'


def forwards_add_host_manage_refresh_interval_config(apps, schema_editor):
    SysConfig = apps.get_model('sys_config', 'SysConfig')
    SysConfig.objects.get_or_create(
        key=HOST_MANAGE_REFRESH_INTERVAL_SECONDS_KEY,
        defaults={
            'value': '5',
            'default_value': '5',
            'value_type': 'int',
            'name': '主机管理页刷新间隔（秒）',
            'description': '主机管理列表自动刷新间隔（秒）',
            'is_readonly': False,
        },
    )


def backwards_remove_host_manage_refresh_interval_config(apps, schema_editor):
    SysConfig = apps.get_model('sys_config', 'SysConfig')
    SysConfig.objects.filter(key=HOST_MANAGE_REFRESH_INTERVAL_SECONDS_KEY).delete()


class Migration(migrations.Migration):

    dependencies = [
        ('sys_config', '0008_add_agent_collect_interval_config'),
    ]

    operations = [
        migrations.RunPython(
            forwards_add_host_manage_refresh_interval_config,
            backwards_remove_host_manage_refresh_interval_config,
        ),
    ]
