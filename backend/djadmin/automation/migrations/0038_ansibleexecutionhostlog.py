from django.db import migrations, models
import django.db.models.deletion


class Migration(migrations.Migration):

    dependencies = [
        ('automation', '0037_remove_automationtask_code'),
        ('assets', '0031_host_agent_online_time'),
    ]

    operations = [
        migrations.CreateModel(
            name='AutomationExecutionTargetLog',
            fields=[
                ('id', models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name='ID')),
                ('remark', models.CharField(blank=True, max_length=255, null=True)),
                ('create_time', models.DateTimeField(auto_now_add=True, null=True)),
                ('update_time', models.DateTimeField(auto_now=True, null=True)),
                ('host_id_snapshot', models.IntegerField(blank=True, null=True)),
                ('host_name_snapshot', models.CharField(blank=True, default='', max_length=128)),
                ('host_ip_snapshot', models.CharField(blank=True, default='', max_length=64)),
                ('agent_job_id', models.CharField(blank=True, default='', max_length=64)),
                ('status', models.CharField(choices=[('pending', 'Pending'), ('running', 'Running'), ('success', 'Success'), ('failed', 'Failed'), ('cancelled', 'Cancelled')], default='failed', max_length=16)),
                ('exit_code', models.IntegerField(blank=True, null=True)),
                ('stdout', models.TextField(blank=True, default='')),
                ('stderr', models.TextField(blank=True, default='')),
                ('error_message', models.TextField(blank=True, default='')),
                ('result_data', models.JSONField(blank=True, default=dict)),
                ('host', models.ForeignKey(blank=True, null=True, on_delete=django.db.models.deletion.SET_NULL, related_name='automation_host_logs', to='assets.host')),
                ('job', models.ForeignKey(on_delete=django.db.models.deletion.CASCADE, related_name='host_logs', to='automation.ansibleexecutionjob')),
            ],
            options={
                'db_table': 'automation_execution_host_log',
                'ordering': ['id'],
            },
        ),
    ]
