from django.db import migrations


def forwards_remove_collect_auth_failure_threshold(apps, schema_editor):
    SysConfig = apps.get_model('sys_config', 'SysConfig')
    SysConfig.objects.filter(key='sys.assets.collect.auth_failure_threshold').delete()


def backwards_restore_collect_auth_failure_threshold(apps, schema_editor):
    SysConfig = apps.get_model('sys_config', 'SysConfig')
    SysConfig.objects.get_or_create(
        key='sys.assets.collect.auth_failure_threshold',
        defaults={
            'value': '3',
            'default_value': '3',
            'value_type': 'int',
            'name': '主机采集认证失败阈值',
            'description': '连续认证失败达到该次数后，主机进入自动采集保护期',
            'is_readonly': False,
        },
    )


class Migration(migrations.Migration):

    dependencies = [
        ('sys_config', '0005_alter_sysconfig_create_time'),
    ]

    operations = [
        migrations.RunPython(
            forwards_remove_collect_auth_failure_threshold,
            backwards_restore_collect_auth_failure_threshold,
        ),
    ]
