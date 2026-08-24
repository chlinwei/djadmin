from django.db import migrations, models
import django.db.models.deletion


def backfill_service_defaults(apps, schema_editor):
    ApplicationService = apps.get_model('assets', 'ApplicationService')
    for service in ApplicationService.objects.filter(
        application_version__isnull=True,
        deployment_template__isnull=True,
    ).prefetch_related('deployments'):
        deployment = service.deployments.order_by('id').first()
        if deployment is None:
            continue
        service.application_version_id = deployment.application_version_id
        service.deployment_template_id = deployment.deployment_template_id
        service.save(update_fields=['application_version', 'deployment_template'])


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0082_alter_applicationdeployment_member_port'),
    ]

    operations = [
        migrations.AddField(
            model_name='applicationservice',
            name='application_version',
            field=models.ForeignKey(
                blank=True,
                null=True,
                on_delete=django.db.models.deletion.PROTECT,
                related_name='services',
                to='assets.applicationversion',
                verbose_name='应用版本',
            ),
        ),
        migrations.AddField(
            model_name='applicationservice',
            name='deployment_template',
            field=models.ForeignKey(
                blank=True,
                null=True,
                on_delete=django.db.models.deletion.PROTECT,
                related_name='services',
                to='assets.applicationdeploymenttemplate',
                verbose_name='部署模板',
            ),
        ),
        migrations.RunPython(backfill_service_defaults, migrations.RunPython.noop),
    ]
