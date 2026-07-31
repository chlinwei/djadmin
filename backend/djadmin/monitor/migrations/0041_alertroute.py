import django.utils.timezone
from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('monitor', '0040_alert_notification_models'),
    ]

    operations = [
        migrations.CreateModel(
            name='AlertRoute',
            fields=[
                ('id', models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name='ID')),
                ('create_time', models.DateTimeField(default=django.utils.timezone.now, verbose_name='创建时间')),
                ('update_time', models.DateTimeField(auto_now=True, verbose_name='修改时间')),
                ('remark', models.TextField(blank=True, default='', null=True, verbose_name='备注')),
                ('name', models.CharField(max_length=128, unique=True)),
                ('enabled', models.BooleanField(default=True)),
                ('matchers', models.JSONField(blank=True, default=dict)),
                ('notify_on_firing', models.BooleanField(default=True)),
                ('notify_on_resolved', models.BooleanField(default=True)),
                ('media', models.ManyToManyField(blank=True, related_name='alert_routes', to='monitor.alertmedia')),
            ],
            options={
                'db_table': 'monitor_alert_route',
                'ordering': ['id'],
            },
        ),
    ]