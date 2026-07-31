from django.db import migrations, models
import django.db.models.deletion
import django.utils.timezone


class Migration(migrations.Migration):

    dependencies = [
        ('monitor', '0031_unify_prometheus_tokens_to_apitoken'),
        ('user', '0014_alter_apitoken_bind_mode_alter_apitoken_created_by'),
    ]

    operations = [
        migrations.CreateModel(
            name='AlertMedia',
            fields=[
                ('id', models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name='ID')),
                ('create_time', models.DateTimeField(default=django.utils.timezone.now, verbose_name='创建时间')),
                ('update_time', models.DateTimeField(auto_now=True, verbose_name='修改时间')),
                ('remark', models.TextField(blank=True, default='', null=True, verbose_name='备注')),
                ('name', models.CharField(max_length=128)),
                ('media_type', models.CharField(choices=[('email', 'Email'), ('webhook', 'Webhook')], max_length=16)),
                ('config', models.JSONField(blank=True, default=dict)),
                ('enabled', models.BooleanField(default=True)),
                ('users', models.ManyToManyField(blank=True, related_name='alert_media', to='user.sysuser')),
            ],
            options={'db_table': 'monitor_alert_media', 'ordering': ['-id']},
        ),
        migrations.CreateModel(
            name='AlertNotificationEvent',
            fields=[
                ('id', models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name='ID')),
                ('create_time', models.DateTimeField(default=django.utils.timezone.now, verbose_name='创建时间')),
                ('update_time', models.DateTimeField(auto_now=True, verbose_name='修改时间')),
                ('remark', models.TextField(blank=True, default='', null=True, verbose_name='备注')),
                ('event_type', models.CharField(max_length=16)),
                ('deduplication_key', models.CharField(max_length=255, unique=True)),
                ('status', models.CharField(choices=[('pending', 'Pending'), ('sending', 'Sending'), ('success', 'Success'), ('failed', 'Failed')], default='pending', max_length=16)),
                ('attempt_count', models.PositiveIntegerField(default=0)),
                ('error_message', models.TextField(blank=True, default='')),
                ('sent_at', models.DateTimeField(blank=True, null=True)),
                ('alert', models.ForeignKey(on_delete=django.db.models.deletion.CASCADE, related_name='notification_events', to='monitor.alerthistory')),
            ],
            options={'db_table': 'monitor_alert_notification_event', 'ordering': ['-id']},
        ),
        migrations.CreateModel(
            name='AlertNotificationDelivery',
            fields=[
                ('id', models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name='ID')),
                ('create_time', models.DateTimeField(default=django.utils.timezone.now, verbose_name='创建时间')),
                ('update_time', models.DateTimeField(auto_now=True, verbose_name='修改时间')),
                ('remark', models.TextField(blank=True, default='', null=True, verbose_name='备注')),
                ('address', models.CharField(max_length=500)),
                ('status', models.CharField(default='pending', max_length=16)),
                ('attempt_count', models.PositiveIntegerField(default=0)),
                ('error_message', models.TextField(blank=True, default='')),
                ('sent_at', models.DateTimeField(blank=True, null=True)),
                ('event', models.ForeignKey(on_delete=django.db.models.deletion.CASCADE, related_name='deliveries', to='monitor.alertnotificationevent')),
                ('media', models.ForeignKey(null=True, on_delete=django.db.models.deletion.SET_NULL, related_name='deliveries', to='monitor.alertmedia')),
                ('user', models.ForeignKey(null=True, on_delete=django.db.models.deletion.SET_NULL, related_name='alert_deliveries', to='user.sysuser')),
            ],
            options={'db_table': 'monitor_alert_notification_delivery'},
        ),
        migrations.AddConstraint(
            model_name='alertmedia',
            constraint=models.UniqueConstraint(fields=('name',), name='monitor_alert_media_name_uniq'),
        ),
        migrations.AddConstraint(
            model_name='alertnotificationdelivery',
            constraint=models.UniqueConstraint(fields=('event', 'media', 'user', 'address'), name='monitor_alert_delivery_uniq'),
        ),
    ]