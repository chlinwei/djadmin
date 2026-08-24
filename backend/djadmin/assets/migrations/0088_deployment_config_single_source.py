from django.db import migrations, models
import django.db.models.deletion


def backfill_service_config(apps, schema_editor):
    """删除实例侧字段前，用成员实例的配置补齐服务侧空值，避免丢失既有版本/模板。"""
    ApplicationService = apps.get_model('assets', 'ApplicationService')
    for service in ApplicationService.objects.filter(
        models.Q(application_version__isnull=True) | models.Q(deployment_template__isnull=True)
    ):
        link = service.deployment_links.select_related('deployment').first()
        deployment = getattr(link, 'deployment', None)
        if deployment is None:
            continue
        if service.application_version_id is None:
            service.application_version_id = deployment.application_version_id
        if service.deployment_template_id is None:
            service.deployment_template_id = deployment.deployment_template_id
        service.save(update_fields=['application_version', 'deployment_template'])


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0087_merge_20260824_1332'),
    ]

    operations = [
        migrations.RunPython(backfill_service_config, migrations.RunPython.noop),
        migrations.AlterField(
            model_name='applicationservice',
            name='application_version',
            field=models.ForeignKey(
                on_delete=django.db.models.deletion.PROTECT,
                related_name='services',
                to='assets.applicationversion',
                verbose_name='应用版本',
            ),
        ),
        migrations.AlterField(
            model_name='applicationservice',
            name='deployment_template',
            field=models.ForeignKey(
                on_delete=django.db.models.deletion.PROTECT,
                related_name='services',
                to='assets.applicationdeploymenttemplate',
                verbose_name='部署模板',
            ),
        ),
        migrations.RemoveField(model_name='applicationdeployment', name='application_version'),
        migrations.RemoveField(model_name='applicationdeployment', name='deployment_template'),
    ]
