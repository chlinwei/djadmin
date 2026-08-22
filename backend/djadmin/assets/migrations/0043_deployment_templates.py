import django.db.models.deletion
import django.utils.timezone
from django.db import migrations, models


def migrate_deployment_configs_to_templates(apps, schema_editor):
    ApplicationDeployment = apps.get_model('assets', 'ApplicationDeployment')
    ApplicationDeploymentTemplate = apps.get_model('assets', 'ApplicationDeploymentTemplate')

    for deployment in ApplicationDeployment.objects.select_related('application_version').iterator():
        template_name = f'{deployment.instance_name} 模板'
        if ApplicationDeploymentTemplate.objects.filter(
            application_id=deployment.application_version.application_id,
            name=template_name,
        ).exists():
            template_name = f'{template_name} ({deployment.pk})'
        template = ApplicationDeploymentTemplate.objects.create(
            id=deployment.pk,
            application_id=deployment.application_version.application_id,
            name=template_name,
            control_type=deployment.control_type,
            run_user=deployment.run_user,
            run_group=deployment.run_group,
            app_home=deployment.app_home,
            work_directory=deployment.work_directory,
            service_name=deployment.service_name,
            ha_system_name=deployment.ha_system_name,
            ha_cluster_name=deployment.ha_cluster_name,
            ha_resource_name=deployment.ha_resource_name,
            enabled=deployment.enabled,
            remark=deployment.remark,
        )
        deployment.deployment_template_id = template.pk
        deployment.save(update_fields=['deployment_template'])


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0042_application_baseline_checks'),
    ]

    operations = [
        migrations.CreateModel(
            name='ApplicationDeploymentTemplate',
            fields=[
                ('id', models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name='ID')),
                ('create_time', models.DateTimeField(default=django.utils.timezone.now, verbose_name='创建时间')),
                ('update_time', models.DateTimeField(auto_now=True, verbose_name='修改时间')),
                ('remark', models.TextField(blank=True, default='', null=True, verbose_name='备注')),
                ('name', models.CharField(max_length=128, verbose_name='模板名称')),
                ('control_type', models.CharField(choices=[('systemd', 'Systemd'), ('command', '命令行'), ('external_ha', '外部 HA'), ('docker', 'Docker 容器'), ('docker_compose', 'Docker Compose')], max_length=32)),
                ('run_user', models.CharField(max_length=100, verbose_name='运行用户')),
                ('run_group', models.CharField(blank=True, default='', max_length=100, verbose_name='运行组')),
                ('app_home', models.CharField(blank=True, default='', max_length=512, verbose_name='App Home')),
                ('work_directory', models.CharField(blank=True, default='', max_length=512, verbose_name='工作目录')),
                ('service_name', models.CharField(blank=True, default='', max_length=255, verbose_name='Systemd 服务名')),
                ('ha_system_name', models.CharField(blank=True, default='', max_length=128, verbose_name='HA 系统名称')),
                ('ha_cluster_name', models.CharField(blank=True, default='', max_length=128, verbose_name='集群名称')),
                ('ha_resource_name', models.CharField(blank=True, default='', max_length=128, verbose_name='资源名称')),
                ('enabled', models.BooleanField(default=True, verbose_name='允许新部署')),
                ('application', models.ForeignKey(on_delete=django.db.models.deletion.CASCADE, related_name='deployment_templates', to='assets.application')),
            ],
            options={
                'db_table': 'assets_application_deployment_template',
                'ordering': ['application_id', '-id'],
                'constraints': [models.UniqueConstraint(fields=('application', 'name'), name='unique_application_deployment_template')],
            },
        ),
        migrations.AddField(
            model_name='applicationdeployment',
            name='deployment_template',
            field=models.ForeignKey(null=True, on_delete=django.db.models.deletion.PROTECT, related_name='deployments', to='assets.applicationdeploymenttemplate'),
        ),
        migrations.RunPython(migrate_deployment_configs_to_templates, migrations.RunPython.noop),
        migrations.RemoveConstraint(model_name='applicationport', name='unique_deployment_protocol_port'),
        migrations.RemoveConstraint(model_name='applicationpath', name='unique_deployment_path_name'),
        migrations.RemoveConstraint(model_name='applicationconfigfile', name='unique_deployment_config_path'),
        migrations.RemoveConstraint(model_name='applicationlogdefinition', name='unique_deployment_log_name'),
        migrations.RemoveConstraint(model_name='applicationcontrolaction', name='unique_deployment_control_action'),
        migrations.RenameField(model_name='applicationport', old_name='deployment', new_name='deployment_template'),
        migrations.RenameField(model_name='applicationpath', old_name='deployment', new_name='deployment_template'),
        migrations.RenameField(model_name='applicationconfigfile', old_name='deployment', new_name='deployment_template'),
        migrations.RenameField(model_name='applicationlogdefinition', old_name='deployment', new_name='deployment_template'),
        migrations.RenameField(model_name='applicationcontrolaction', old_name='deployment', new_name='deployment_template'),
        migrations.RenameField(model_name='dockercontrolconfig', old_name='deployment', new_name='deployment_template'),
        migrations.RenameField(model_name='dockercomposecontrolconfig', old_name='deployment', new_name='deployment_template'),
        migrations.AlterField(
            model_name='applicationport', name='deployment_template',
            field=models.ForeignKey(on_delete=django.db.models.deletion.CASCADE, related_name='ports', to='assets.applicationdeploymenttemplate'),
        ),
        migrations.AlterField(
            model_name='applicationpath', name='deployment_template',
            field=models.ForeignKey(on_delete=django.db.models.deletion.CASCADE, related_name='paths', to='assets.applicationdeploymenttemplate'),
        ),
        migrations.AlterField(
            model_name='applicationconfigfile', name='deployment_template',
            field=models.ForeignKey(on_delete=django.db.models.deletion.CASCADE, related_name='config_files', to='assets.applicationdeploymenttemplate'),
        ),
        migrations.AlterField(
            model_name='applicationlogdefinition', name='deployment_template',
            field=models.ForeignKey(on_delete=django.db.models.deletion.CASCADE, related_name='logs', to='assets.applicationdeploymenttemplate'),
        ),
        migrations.AlterField(
            model_name='applicationcontrolaction', name='deployment_template',
            field=models.ForeignKey(on_delete=django.db.models.deletion.CASCADE, related_name='control_actions', to='assets.applicationdeploymenttemplate'),
        ),
        migrations.AlterField(
            model_name='dockercontrolconfig', name='deployment_template',
            field=models.OneToOneField(on_delete=django.db.models.deletion.CASCADE, related_name='docker_config', to='assets.applicationdeploymenttemplate'),
        ),
        migrations.AlterField(
            model_name='dockercomposecontrolconfig', name='deployment_template',
            field=models.OneToOneField(on_delete=django.db.models.deletion.CASCADE, related_name='compose_config', to='assets.applicationdeploymenttemplate'),
        ),
        migrations.AddConstraint(
            model_name='applicationport',
            constraint=models.UniqueConstraint(fields=('deployment_template', 'protocol', 'port'), name='unique_template_protocol_port'),
        ),
        migrations.AddConstraint(
            model_name='applicationpath',
            constraint=models.UniqueConstraint(fields=('deployment_template', 'name'), name='unique_template_path_name'),
        ),
        migrations.AddConstraint(
            model_name='applicationconfigfile',
            constraint=models.UniqueConstraint(fields=('deployment_template', 'path'), name='unique_template_config_path'),
        ),
        migrations.AddConstraint(
            model_name='applicationlogdefinition',
            constraint=models.UniqueConstraint(fields=('deployment_template', 'name'), name='unique_template_log_name'),
        ),
        migrations.AddConstraint(
            model_name='applicationcontrolaction',
            constraint=models.UniqueConstraint(fields=('deployment_template', 'action'), name='unique_template_control_action'),
        ),
        migrations.AlterModelOptions(name='applicationport', options={'ordering': ['deployment_template_id', 'protocol', 'port']}),
        migrations.AlterModelOptions(name='applicationpath', options={'ordering': ['deployment_template_id', 'path_type', 'id']}),
        migrations.AlterModelOptions(name='applicationconfigfile', options={'ordering': ['deployment_template_id', 'id']}),
        migrations.AlterModelOptions(name='applicationlogdefinition', options={'ordering': ['deployment_template_id', 'id']}),
        migrations.AlterModelOptions(name='applicationcontrolaction', options={'ordering': ['deployment_template_id', 'id']}),
        migrations.RemoveField(model_name='applicationdeployment', name='control_type'),
        migrations.RemoveField(model_name='applicationdeployment', name='run_user'),
        migrations.RemoveField(model_name='applicationdeployment', name='run_group'),
        migrations.RemoveField(model_name='applicationdeployment', name='app_home'),
        migrations.RemoveField(model_name='applicationdeployment', name='work_directory'),
        migrations.RemoveField(model_name='applicationdeployment', name='service_name'),
        migrations.RemoveField(model_name='applicationdeployment', name='ha_system_name'),
        migrations.RemoveField(model_name='applicationdeployment', name='ha_cluster_name'),
        migrations.RemoveField(model_name='applicationdeployment', name='ha_resource_name'),
        migrations.AlterField(
            model_name='applicationdeployment', name='deployment_template',
            field=models.ForeignKey(on_delete=django.db.models.deletion.PROTECT, related_name='deployments', to='assets.applicationdeploymenttemplate'),
        ),
    ]
