import django.utils.timezone
from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0025_agentjob_client_request_id'),
    ]

    operations = [
        migrations.CreateModel(
            name='AgentJobEvent',
            fields=[
                ('id', models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name='ID')),
                ('create_time', models.DateTimeField(default=django.utils.timezone.now, verbose_name='创建时间')),
                ('update_time', models.DateTimeField(auto_now=True, verbose_name='修改时间')),
                ('remark', models.TextField(blank=True, default='', null=True, verbose_name='备注')),
                ('tag', models.CharField(max_length=255)),
                ('job_id', models.CharField(blank=True, default='', max_length=128)),
                ('agent_id', models.CharField(blank=True, default='', max_length=128)),
                ('event_type', models.CharField(blank=True, default='', max_length=64)),
                ('payload', models.JSONField(blank=True, default=dict)),
            ],
            options={
                'db_table': 'assets_agent_job_event',
                'indexes': [
                    models.Index(fields=['job_id', 'create_time'], name='assets_agen_job_id__7d7de9_idx'),
                    models.Index(fields=['agent_id', 'create_time'], name='assets_agen_agent_i_16ed57_idx'),
                    models.Index(fields=['tag', 'create_time'], name='assets_agen_tag_6f8758_idx'),
                ],
            },
        ),
    ]
