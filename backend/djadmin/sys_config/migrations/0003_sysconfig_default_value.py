from django.db import migrations, models


def set_default_value_for_existing(apps, schema_editor):
    SysConfig = apps.get_model('sys_config', 'SysConfig')
    for item in SysConfig.objects.all().only('id', 'value', 'default_value'):
        if item.default_value is None:
            item.default_value = item.value
            item.save(update_fields=['default_value'])


class Migration(migrations.Migration):

    dependencies = [
        ('sys_config', '0002_alter_sysconfig_update_time'),
    ]

    operations = [
        migrations.AddField(
            model_name='sysconfig',
            name='default_value',
            field=models.TextField(blank=True, null=True, verbose_name='默认值'),
        ),
        migrations.RunPython(set_default_value_for_existing, migrations.RunPython.noop),
    ]
