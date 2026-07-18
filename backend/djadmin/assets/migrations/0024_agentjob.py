import django.utils.timezone
from django.db import migrations, models
import django.db.models.deletion


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0023_backfill_hostsystem_collector_source'),
    ]

    operations = [
        migrations.CreateModel(
            name='AgentJob',
            fields=[
                ('id', models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name='ID')),
                ('create_time', models.DateTimeField(default=django.utils.timezone.now, verbose_name='创建时间')),
                ('update_time', models.DateTimeField(auto_now=True, verbose_name='修改时间')),
                ('remark', models.TextField(blank=True, default='', null=True, verbose_name='备注')),
                ('job_id', models.CharField(max_length=128, unique=True)),
                ('agent_id', models.CharField(max_length=128)),
                ('job_type', models.CharField(max_length=32)),
                ('action', models.CharField(max_length=64)),
                ('params', models.JSONField(blank=True, default=dict)),
                ('timeout_seconds', models.PositiveIntegerField(default=30)),
                ('status', models.CharField(choices=[('queued', 'Queued'), ('running', 'Running'), ('success', 'Success'), ('failed', 'Failed'), ('canceled', 'Canceled'), ('timeout', 'Timeout')], default='queued', max_length=16)),
                ('picked_at', models.DateTimeField(blank=True, null=True)),
                ('finished_at', models.DateTimeField(blank=True, null=True)),
                ('result_data', models.JSONField(blank=True, default=dict)),
                ('error_message', models.TextField(blank=True, default='')),
                ('host', models.ForeignKey(blank=True, null=True, on_delete=django.db.models.deletion.SET_NULL, related_name='agent_jobs', to='assets.host')),
            ],
            options={
                'db_table': 'assets_agent_job',
                'indexes': [models.Index(fields=['agent_id', 'status'], name='assets_agen_agent_i_2583a5_idx'), models.Index(fields=['status', 'create_time'], name='assets_agen_status_4fc5ab_idx')],
            },
        ),
    ]
