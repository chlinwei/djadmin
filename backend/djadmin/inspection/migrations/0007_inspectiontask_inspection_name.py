from django.db import migrations, models


def initialize_scheduled_inspection_names(apps, schema_editor):
    inspection_task = apps.get_model('inspection', 'InspectionTask')
    inspection_task.objects.exclude(cron_expression='').update(inspection_name=models.F('name'))


class Migration(migrations.Migration):

    dependencies = [
        ('inspection', '0006_inspectioncheck_severity_and_more'),
    ]

    operations = [
        migrations.AddField(
            model_name='inspectiontask',
            name='inspection_name',
            field=models.CharField(blank=True, default='', max_length=128, verbose_name='巡检名称'),
        ),
        migrations.RunPython(initialize_scheduled_inspection_names, migrations.RunPython.noop),
    ]