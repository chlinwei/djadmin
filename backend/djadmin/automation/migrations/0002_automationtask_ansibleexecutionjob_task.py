import datetime
from django.db import migrations, models
import django.db.models.deletion


class Migration(migrations.Migration):

    dependencies = [
        ('automation', '0001_initial'),
    ]

    operations = [
        migrations.CreateModel(
            name='AutomationTask',
            fields=[
                ('id', models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name='ID')),
                ('create_time', models.DateField(default=datetime.date(2026, 7, 2), verbose_name='创建时间')),
                ('update_time', models.DateField(auto_now=True, verbose_name='修改时间')),
                ('remark', models.TextField(blank=True, default='', null=True, verbose_name='备注')),
                ('name', models.CharField(max_length=128, unique=True)),
                ('code', models.CharField(max_length=128, unique=True)),
                ('selected_host_ids', models.JSONField(blank=True, default=list)),
                ('selected_group_ids', models.JSONField(blank=True, default=list)),
                ('env_vars', models.JSONField(blank=True, default=dict)),
                ('enabled', models.BooleanField(default=True)),
                ('template', models.ForeignKey(on_delete=django.db.models.deletion.PROTECT, related_name='tasks', to='automation.playbooktemplate')),
            ],
            options={
                'db_table': 'automation_task',
                'ordering': ['-id'],
            },
        ),
        migrations.AddField(
            model_name='ansibleexecutionjob',
            name='task',
            field=models.ForeignKey(blank=True, null=True, on_delete=django.db.models.deletion.SET_NULL, related_name='jobs', to='automation.automationtask'),
        ),
    ]
