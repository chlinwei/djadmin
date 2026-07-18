from django.db import migrations


def forwards_remove_collect_auth_lock_minutes(apps, schema_editor):
    SysConfig = apps.get_model('sys_config', 'SysConfig')
    SysConfig.objects.filter(key='sys.assets.collect.auth_lock_minutes').delete()


def backwards_restore_collect_auth_lock_minutes(apps, schema_editor):
    SysConfig = apps.get_model('sys_config', 'SysConfig')
    SysConfig.objects.get_or_create(
        key='sys.assets.collect.auth_lock_minutes',
        defaults={
            'value': '30',
            'default_value': '30',
            'value_type': 'int',
            'name': '主机采集认证保护时长（分钟）',
            'description': '进入保护期后，定时采集跳过该主机的时长（分钟）',
            'is_readonly': False,
        },
    )


class Migration(migrations.Migration):

    dependencies = [
        ('sys_config', '0006_remove_collect_auth_failure_threshold'),
    ]

    operations = [
        migrations.RunPython(
            forwards_remove_collect_auth_lock_minutes,
            backwards_restore_collect_auth_lock_minutes,
        ),
    ]
