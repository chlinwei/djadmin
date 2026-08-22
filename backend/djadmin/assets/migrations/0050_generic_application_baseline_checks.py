import django.db.models.deletion
import django.utils.timezone
from django.db import migrations, models


def migrate_tomcat_checks(apps, schema_editor):
    TomcatBaselineConfig = apps.get_model('assets', 'TomcatBaselineConfig')
    ApplicationBaselineCheck = apps.get_model('assets', 'ApplicationBaselineCheck')

    for config in TomcatBaselineConfig.objects.all().iterator():
        common = {'application_id': config.application_id, 'file_type': 'xml', 'enabled': True}
        ApplicationBaselineCheck.objects.create(
            **common,
            name='Tomcat Connector maxPostSize',
            file_path=config.server_xml_path,
            order=10,
            rule={
                'element': 'Connector',
                'match': {'protocol': {'operator': 'contains_ci', 'value': 'http'}},
                'assertion': 'attributes',
                'attributes': {
                    'maxPostSize': {
                        'operator': 'gte_number',
                        'value': config.max_post_size_mb * 1024 * 1024,
                        'default': 50 * 1024 * 1024,
                        'unlimited_values': ['-1'],
                    },
                },
            },
        )
        ApplicationBaselineCheck.objects.create(
            **common,
            name='Tomcat Manager 登录用户',
            file_path=config.users_xml_path,
            order=20,
            rule={
                'element': 'user',
                'match': {'username': {'operator': 'eq', 'value': config.manager_username}},
                'assertion': 'attributes',
                'attributes': {
                    'password': {'operator': 'eq', 'value': config.manager_password, 'sensitive': True},
                    'roles': {'operator': 'csv_contains_all', 'value': config.required_roles},
                },
            },
        )
        ApplicationBaselineCheck.objects.create(
            **common,
            name='Tomcat Manager 默认 RemoteAddrValve 不存在',
            file_path=config.manager_context_xml_path,
            order=30,
            rule={
                'element': 'Valve',
                'match': {
                    'className': {'operator': 'eq', 'value': 'org.apache.catalina.valves.RemoteAddrValve'},
                    'allow': {'operator': 'eq', 'value': '^.*$'},
                },
                'assertion': 'absent',
                'attributes': {},
            },
        )


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0049_move_tomcat_baseline_to_application'),
    ]

    operations = [
        migrations.CreateModel(
            name='ApplicationBaselineCheck',
            fields=[
                ('id', models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name='ID')),
                ('create_time', models.DateTimeField(default=django.utils.timezone.now, verbose_name='创建时间')),
                ('update_time', models.DateTimeField(auto_now=True, verbose_name='修改时间')),
                ('remark', models.TextField(blank=True, default='', null=True, verbose_name='备注')),
                ('name', models.CharField(max_length=128, verbose_name='检查项名称')),
                ('file_path', models.CharField(max_length=512, verbose_name='文件路径')),
                ('file_type', models.CharField(choices=[('xml', 'XML')], default='xml', max_length=16)),
                ('rule', models.JSONField(default=dict, verbose_name='检查规则')),
                ('enabled', models.BooleanField(default=True)),
                ('order', models.PositiveIntegerField(default=0)),
                ('application', models.ForeignKey(on_delete=django.db.models.deletion.CASCADE, related_name='baseline_checks', to='assets.application')),
            ],
            options={
                'db_table': 'assets_application_baseline_check',
                'ordering': ['order', 'id'],
            },
        ),
        migrations.AddConstraint(
            model_name='applicationbaselinecheck',
            constraint=models.UniqueConstraint(fields=('application', 'name'), name='unique_application_baseline_check_name'),
        ),
        migrations.RunPython(migrate_tomcat_checks, migrations.RunPython.noop),
        migrations.RemoveField(model_name='application', name='baseline_type'),
        migrations.DeleteModel(name='TomcatBaselineConfig'),
    ]