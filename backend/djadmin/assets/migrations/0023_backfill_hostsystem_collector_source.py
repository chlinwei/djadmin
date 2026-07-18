from django.db import migrations


def forward_fill_collector_source(apps, schema_editor):
    HostSystem = apps.get_model('assets', 'HostSystem')

    for row in HostSystem.objects.all().only('id', 'agent_version', 'collector_source'):
        if row.collector_source:
            continue

        version_text = (row.agent_version or '').strip().lower()
        if not version_text:
            continue

        if version_text.startswith('dj_agent:'):
            row.collector_source = 'agent'
        elif version_text == 'ssh-collector':
            row.collector_source = 'ssh'
        else:
            # 旧数据默认按 ssh 采集来源处理，避免来源字段长期为空。
            row.collector_source = 'ssh'
        row.save(update_fields=['collector_source'])


def backward_clear_collector_source(apps, schema_editor):
    HostSystem = apps.get_model('assets', 'HostSystem')
    HostSystem.objects.update(collector_source=None)


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0022_hostsystem_collector_source'),
    ]

    operations = [
        migrations.RunPython(forward_fill_collector_source, backward_clear_collector_source),
    ]
