from django.db import migrations


def remove_legacy_alert_rules_yaml_config(apps, schema_editor):
    SysConfig = apps.get_model('sys_config', 'SysConfig')
    SysConfig.objects.filter(key='sys.monitor.prometheus.alert_rules_yaml').delete()
    SysConfig.objects.filter(key='monitor.prometheus.alert_rules_yaml').delete()


class Migration(migrations.Migration):

    dependencies = [
        ('monitor', '0024_add_remark_to_alert_rule'),
        ('sys_config', '0011_add_automation_websocket_poll_interval_configs'),
    ]

    operations = [
        migrations.RunPython(remove_legacy_alert_rules_yaml_config, migrations.RunPython.noop),
    ]
