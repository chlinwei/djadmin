from django.db import migrations, models
import django.db.models.deletion


def move_tomcat_configs_to_applications(apps, schema_editor):
    Application = apps.get_model('assets', 'Application')
    TomcatBaselineConfig = apps.get_model('assets', 'TomcatBaselineConfig')

    application_ids = TomcatBaselineConfig.objects.values_list(
        'deployment_template__application_id', flat=True
    ).distinct()
    for application_id in application_ids:
        configs = TomcatBaselineConfig.objects.filter(
            deployment_template__application_id=application_id,
        ).order_by('-enabled', '-id')
        selected = configs.first()
        if selected is None or not selected.enabled:
            configs.delete()
            continue
        configs.exclude(id=selected.id).delete()
        selected.application_id = application_id
        selected.save(update_fields=['application'])
        Application.objects.filter(id=application_id).update(baseline_type='tomcat')


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0048_alter_tomcatbaselineconfig_create_time_and_more'),
    ]

    operations = [
        migrations.AddField(
            model_name='application',
            name='baseline_type',
            field=models.CharField(
                choices=[('none', '不使用应用基线'), ('tomcat', 'Tomcat')],
                default='none',
                max_length=32,
                verbose_name='应用基线类型',
            ),
        ),
        migrations.AddField(
            model_name='tomcatbaselineconfig',
            name='application',
            field=models.OneToOneField(
                null=True,
                on_delete=django.db.models.deletion.CASCADE,
                related_name='tomcat_baseline',
                to='assets.application',
            ),
        ),
        migrations.RunPython(move_tomcat_configs_to_applications, migrations.RunPython.noop),
        migrations.RemoveField(
            model_name='tomcatbaselineconfig',
            name='deployment_template',
        ),
        migrations.RemoveField(
            model_name='tomcatbaselineconfig',
            name='enabled',
        ),
        migrations.AlterField(
            model_name='tomcatbaselineconfig',
            name='application',
            field=models.OneToOneField(
                on_delete=django.db.models.deletion.CASCADE,
                related_name='tomcat_baseline',
                to='assets.application',
            ),
        ),
    ]
