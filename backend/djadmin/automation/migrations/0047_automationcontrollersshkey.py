import django.utils.timezone
from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('automation', '0046_remove_workflow_entry_node_key'),
    ]

    operations = [
        migrations.CreateModel(
            name='AutomationControllerSSHKey',
            fields=[
                ('id', models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name='ID')),
                ('create_time', models.DateTimeField(default=django.utils.timezone.now, verbose_name='创建时间')),
                ('update_time', models.DateTimeField(auto_now=True, verbose_name='修改时间')),
                ('remark', models.TextField(blank=True, default='', null=True, verbose_name='备注')),
                ('public_key', models.CharField(max_length=255, unique=True)),
                ('private_key', models.TextField()),
            ],
            options={
                'db_table': 'automation_controller_ssh_key',
                'ordering': ['id'],
            },
        ),
    ]