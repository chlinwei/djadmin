from django.db import migrations, models


def backfill_required_roles(apps, schema_editor):
    TomcatBaselineConfig = apps.get_model('assets', 'TomcatBaselineConfig')
    default_roles = [
        'manager',
        'manager-gui',
        'admin',
        'admin-gui',
        'manager-script',
        'manager-jmx',
        'manager-status',
    ]
    for item in TomcatBaselineConfig.objects.filter(required_roles=[]).iterator():
        item.required_roles = default_roles
        item.save(update_fields=['required_roles'])


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0045_host_agent_id'),
    ]

    operations = [
        migrations.CreateModel(
            name='TomcatBaselineConfig',
            fields=[
                ('id', models.AutoField(auto_created=True, primary_key=True, serialize=False, verbose_name='ID')),
                ('create_time', models.DateTimeField(auto_now_add=True)),
                ('update_time', models.DateTimeField(auto_now=True)),
                ('remark', models.CharField(blank=True, max_length=255, null=True)),
                ('server_xml_path', models.CharField(blank=True, default='${APP_HOME}/conf/server.xml', max_length=512)),
                ('users_xml_path', models.CharField(blank=True, default='${APP_HOME}/conf/tomcat-users.xml', max_length=512)),
                ('max_post_size_mb', models.PositiveIntegerField(default=500)),
                ('manager_username', models.CharField(blank=True, default='admin', max_length=128)),
                ('manager_password', models.CharField(blank=True, default='', max_length=255)),
                ('required_roles', models.JSONField(blank=True, default=list)),
                ('enabled', models.BooleanField(default=False)),
                (
                    'deployment_template',
                    models.OneToOneField(on_delete=models.deletion.CASCADE, related_name='tomcat_baseline', to='assets.applicationdeploymenttemplate'),
                ),
            ],
            options={
                'db_table': 'assets_tomcat_baseline_config',
            },
        ),
        migrations.RunPython(backfill_required_roles, migrations.RunPython.noop),
    ]
