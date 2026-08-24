from django.db import migrations, models
import django.core.validators
import django.db.models.deletion
from django.utils import timezone


def copy_legacy_links(apps, schema_editor):
    ApplicationService = apps.get_model('assets', 'ApplicationService')
    ApplicationDeployment = apps.get_model('assets', 'ApplicationDeployment')
    Link = apps.get_model('assets', 'ApplicationServiceDeployment')
    for deployment in ApplicationDeployment.objects.exclude(application_service_id=None).iterator():
        Link.objects.create(
            service_id=deployment.application_service_id,
            deployment_id=deployment.id,
            application_port=deployment.member_port,
            enabled=deployment.enabled,
        )


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0083_applicationservice_version_template'),
    ]

    operations = [
        migrations.CreateModel(
            name='ApplicationServiceDeployment',
            fields=[
                ('id', models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name='ID')),
                ('create_time', models.DateTimeField(default=timezone.now, verbose_name='创建时间')),
                ('update_time', models.DateTimeField(auto_now=True, verbose_name='更新时间')),
                ('remark', models.TextField(blank=True, default='', null=True, verbose_name='备注')),
                ('application_port', models.PositiveIntegerField(blank=True, null=True, validators=[django.core.validators.MinValueValidator(1), django.core.validators.MaxValueValidator(65535)], verbose_name='应用端口')),
                ('enabled', models.BooleanField(default=True, verbose_name='是否启用')),
                ('deployment', models.ForeignKey(on_delete=django.db.models.deletion.CASCADE, related_name='service_links', to='assets.applicationdeployment')),
                ('service', models.ForeignKey(on_delete=django.db.models.deletion.CASCADE, related_name='deployment_links', to='assets.applicationservice')),
            ],
            options={'db_table': 'assets_application_service_deployment'},
        ),
        migrations.AddConstraint(
            model_name='applicationservicedeployment',
            constraint=models.UniqueConstraint(fields=('service', 'deployment'), name='unique_application_service_deployment'),
        ),
        migrations.RunPython(copy_legacy_links, migrations.RunPython.noop),
        migrations.AddField(
            model_name='applicationservice',
            name='deployments',
            field=models.ManyToManyField(blank=True, related_name='application_services', through='assets.ApplicationServiceDeployment', to='assets.applicationdeployment', verbose_name='部署实例'),
        ),
        migrations.RemoveField(model_name='applicationdeployment', name='application_service'),
        migrations.RemoveField(model_name='applicationdeployment', name='member_port'),
    ]
