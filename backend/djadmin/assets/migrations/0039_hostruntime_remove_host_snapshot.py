from django.db import migrations, models
import django.db.models.deletion
import django.utils.timezone
from django.utils.dateparse import parse_datetime


def migrate_runtime_snapshot(apps, schema_editor):
    Host = apps.get_model('assets', 'Host')
    HostRuntime = apps.get_model('assets', 'HostRuntime')
    for host in Host.objects.exclude(host_snapshot={}):
        snapshot = host.host_snapshot
        if not isinstance(snapshot, dict):
            continue
        boot_time = parse_datetime(str(snapshot.get('os_boot_time') or '').strip())
        HostRuntime.objects.update_or_create(
            host_id=host.id,
            defaults={
                'cpu_usage_percent': snapshot.get('cpu_usage_percent'),
                'cpu_times': snapshot.get('cpu_times') if isinstance(snapshot.get('cpu_times'), dict) else {},
                'memory_usage_percent': snapshot.get('memory_usage_percent'),
                'memory': snapshot.get('memory') if isinstance(snapshot.get('memory'), dict) else {},
                'disk_io': snapshot.get('disk_io') if isinstance(snapshot.get('disk_io'), (list, dict)) else [],
                'os_uptime_seconds': snapshot.get('os_uptime_seconds'),
                'os_boot_time': boot_time,
                'metrics_sample_window_ms': snapshot.get('metrics_sample_window_ms'),
                'static_fingerprint': str(snapshot.get('_static_fingerprint') or ''),
                'collected_at': host.collect_time,
            },
        )


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0038_deduplicate_host_snapshot'),
    ]

    operations = [
        migrations.CreateModel(
            name='HostRuntime',
            fields=[
                ('id', models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name='ID')),
                ('create_time', models.DateTimeField(default=django.utils.timezone.now, verbose_name='创建时间')),
                ('update_time', models.DateTimeField(auto_now=True, verbose_name='修改时间')),
                ('remark', models.TextField(blank=True, default='', null=True, verbose_name='备注')),
                ('cpu_usage_percent', models.FloatField(blank=True, null=True)),
                ('cpu_times', models.JSONField(blank=True, default=dict)),
                ('memory_usage_percent', models.FloatField(blank=True, null=True)),
                ('memory', models.JSONField(blank=True, default=dict)),
                ('disk_io', models.JSONField(blank=True, default=list)),
                ('os_uptime_seconds', models.BigIntegerField(blank=True, null=True)),
                ('os_boot_time', models.DateTimeField(blank=True, null=True)),
                ('metrics_sample_window_ms', models.PositiveIntegerField(blank=True, null=True)),
                ('static_fingerprint', models.CharField(blank=True, default='', max_length=64)),
                ('collected_at', models.DateTimeField(blank=True, null=True)),
                ('host', models.OneToOneField(on_delete=django.db.models.deletion.CASCADE, related_name='runtime', to='assets.host')),
            ],
        ),
        migrations.RunPython(migrate_runtime_snapshot, migrations.RunPython.noop),
        migrations.RemoveField(
            model_name='host',
            name='host_snapshot',
        ),
    ]