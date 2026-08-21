from django.db import migrations


STRUCTURED_ASSET_FIELDS = {
    'os',
    'os_type',
    'os_version',
    'kernel_version',
    'hostname',
    'agent_version',
    'cpu_count',
    'cpu_model',
    'memory_total_gb',
    'arch',
    'disks',
    'os_timezone',
    'os_utc_offset',
}


def remove_structured_fields_from_snapshots(apps, schema_editor):
    Host = apps.get_model('assets', 'Host')
    for host in Host.objects.exclude(host_snapshot={}):
        snapshot = host.host_snapshot
        if not isinstance(snapshot, dict):
            continue
        cleaned_snapshot = {
            key: value
            for key, value in snapshot.items()
            if key not in STRUCTURED_ASSET_FIELDS
        }
        if cleaned_snapshot != snapshot:
            host.host_snapshot = cleaned_snapshot
            host.save(update_fields=['host_snapshot'])


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0037_hostsystem_timezone'),
    ]

    operations = [
        migrations.RunPython(remove_structured_fields_from_snapshots, migrations.RunPython.noop),
    ]