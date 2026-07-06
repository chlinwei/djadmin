from django.db import migrations, models
import django.db.models.deletion


class Migration(migrations.Migration):

    dependencies = [
        ('automation', '0014_alter_ansibleexecutionjob_create_time_and_more'),
    ]

    operations = [
        migrations.CreateModel(
            name='AutomationWorkflowTemplate',
            fields=[
                ('id', models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name='ID')),
                ('create_time', models.DateField(verbose_name='创建时间')),
                ('update_time', models.DateField(auto_now=True, verbose_name='修改时间')),
                ('remark', models.TextField(blank=True, default='', null=True, verbose_name='备注')),
                ('name', models.CharField(max_length=128, unique=True)),
                ('code', models.CharField(max_length=128, unique=True)),
                ('description', models.CharField(blank=True, default='', max_length=255)),
                ('enabled', models.BooleanField(default=True)),
                ('entry_node_key', models.CharField(max_length=128)),
                ('nodes', models.JSONField(blank=True, default=list)),
                ('edges', models.JSONField(blank=True, default=list)),
                ('default_extra_vars', models.JSONField(blank=True, default=dict)),
            ],
            options={
                'db_table': 'automation_workflow_template',
                'ordering': ['-id'],
            },
        ),
        migrations.CreateModel(
            name='AutomationWorkflowRun',
            fields=[
                ('id', models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name='ID')),
                ('create_time', models.DateField(verbose_name='创建时间')),
                ('update_time', models.DateField(auto_now=True, verbose_name='修改时间')),
                ('remark', models.TextField(blank=True, default='', null=True, verbose_name='备注')),
                ('status', models.CharField(choices=[('pending', 'Pending'), ('running', 'Running'), ('waiting_approval', 'Waiting Approval'), ('success', 'Success'), ('failed', 'Failed')], default='pending', max_length=24)),
                ('trigger_type', models.CharField(choices=[('manual', 'Manual'), ('schedule', 'Schedule')], default='manual', max_length=16)),
                ('workflow_name_snapshot', models.CharField(blank=True, default='', max_length=128)),
                ('workflow_code_snapshot', models.CharField(blank=True, default='', max_length=128)),
                ('planned_node_keys', models.JSONField(blank=True, default=list)),
                ('node_results', models.JSONField(blank=True, default=list)),
                ('extra_vars', models.JSONField(blank=True, default=dict)),
                ('result_summary', models.JSONField(blank=True, default=dict)),
                ('requested_user_id', models.IntegerField(blank=True, null=True)),
                ('requested_username', models.CharField(blank=True, default='', max_length=100)),
                ('start_time', models.DateTimeField(blank=True, null=True)),
                ('end_time', models.DateTimeField(blank=True, null=True)),
                ('duration_seconds', models.FloatField(blank=True, null=True)),
                ('workflow', models.ForeignKey(on_delete=django.db.models.deletion.CASCADE, related_name='runs', to='automation.automationworkflowtemplate')),
            ],
            options={
                'db_table': 'automation_workflow_run',
                'ordering': ['-id'],
            },
        ),
    ]
