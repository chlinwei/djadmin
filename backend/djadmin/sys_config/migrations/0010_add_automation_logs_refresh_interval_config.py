from django.db import migrations


def create_automation_logs_refresh_interval_config(apps, schema_editor):
    SysConfig = apps.get_model('sys_config', 'SysConfig')
    SysConfig.objects.get_or_create(
        key='sys.automation.logs.refresh_interval_seconds',
        defaults={
            'value': '5',
            'default_value': '5',
            'value_type': 'int',
            'name': '运行记录中心刷新间隔（秒）',
            'description': '自动化运行记录中心列表自动刷新间隔（秒）',
            'is_readonly': False,
        },
    )


def reverse_noop(apps, schema_editor):
    # Keep user-managed sys config rows on rollback.
    return


class Migration(migrations.Migration):

    dependencies = [
        ('sys_config', '0009_add_host_manage_refresh_interval_config'),
    ]

    operations = [
        migrations.RunPython(create_automation_logs_refresh_interval_config, reverse_noop),
    ]
