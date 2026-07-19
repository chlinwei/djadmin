from django.db import migrations


def create_automation_websocket_poll_interval_configs(apps, schema_editor):
    SysConfig = apps.get_model('sys_config', 'SysConfig')
    SysConfig.objects.get_or_create(
        key='sys.automation.websocket.job_log_poll_interval_seconds',
        defaults={
            'value': '0.5',
            'default_value': '0.5',
            'value_type': 'string',
            'name': '自动化作业日志WS轮询间隔（秒）',
            'description': '自动化作业日志 WebSocket 拉取后端增量的轮询间隔（秒）',
            'is_readonly': False,
        },
    )
    SysConfig.objects.get_or_create(
        key='sys.automation.websocket.workflow_run_poll_interval_seconds',
        defaults={
            'value': '0.5',
            'default_value': '0.5',
            'value_type': 'string',
            'name': '工作流运行状态WS轮询间隔（秒）',
            'description': '工作流运行状态 WebSocket 拉取后端状态的轮询间隔（秒）',
            'is_readonly': False,
        },
    )


def reverse_noop(apps, schema_editor):
    return


class Migration(migrations.Migration):

    dependencies = [
        ('sys_config', '0010_add_automation_logs_refresh_interval_config'),
    ]

    operations = [
        migrations.RunPython(create_automation_websocket_poll_interval_configs, reverse_noop),
    ]
