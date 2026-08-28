from django.db import migrations, models


def migrate_scope_forward(apps, schema_editor):
    inspection_group = apps.get_model('inspection', 'InspectionGroup')
    inspection_group.objects.filter(scope='controller_once').update(scope='service_once')


def migrate_scope_backward(apps, schema_editor):
    inspection_group = apps.get_model('inspection', 'InspectionGroup')
    inspection_group.objects.filter(scope='service_once').update(scope='controller_once')


class Migration(migrations.Migration):
    dependencies = [
        ('inspection', '0003_inspectionexecution_canceled_status'),
    ]

    operations = [
        migrations.RunPython(migrate_scope_forward, migrate_scope_backward),
        migrations.AlterField(
            model_name='inspectiongroup',
            name='scope',
            field=models.CharField(
                choices=[('per_deployment', '每个部署实例'), ('service_once', '服务单次')],
                max_length=24,
                verbose_name='执行范围',
            ),
        ),
        migrations.AlterField(
            model_name='inspectioncheck',
            name='executor',
            field=models.CharField(
                choices=[('shell', 'Agent Shell'), ('http', 'Agent HTTP'), ('tcp', 'Agent TCP')],
                max_length=16,
                verbose_name='执行器',
            ),
        ),
    ]