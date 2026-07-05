import datetime

from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('automation', '0008_ansibleexecutionjob_task_name_snapshot_and_more'),
    ]

    operations = [
        migrations.CreateModel(
            name='AutomationInventory',
            fields=[
                ('id', models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name='ID')),
                ('create_time', models.DateField(default=datetime.date.today, verbose_name='创建时间')),
                ('update_time', models.DateField(auto_now=True, verbose_name='修改时间')),
                ('remark', models.TextField(blank=True, default='', null=True, verbose_name='备注')),
                ('name', models.CharField(max_length=128, unique=True)),
                ('selected_host_ids', models.JSONField(blank=True, default=list)),
                ('selected_group_ids', models.JSONField(blank=True, default=list)),
                ('enabled', models.BooleanField(default=True)),
            ],
            options={
                'db_table': 'automation_inventory',
                'ordering': ['-id'],
            },
        ),
    ]
