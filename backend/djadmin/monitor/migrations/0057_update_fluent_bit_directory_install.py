from importlib import import_module

from django.db import migrations


def update_install_template(apps, schema_editor):
    playbook_module = import_module('monitor.migrations.0056_bind_fluent_bit_playbooks')
    PlaybookTemplate = apps.get_model('automation', 'PlaybookTemplate')
    PlaybookTemplate.objects.filter(
        name=playbook_module.INSTALL_TEMPLATE_NAME,
    ).update(
        content=playbook_module.INSTALL_PLAYBOOK,
        description='从 djadmin 平台目录传输并离线安装 Fluent Bit RPM/DEB',
    )


class Migration(migrations.Migration):

    dependencies = [
        ('monitor', '0056_bind_fluent_bit_playbooks'),
    ]

    operations = [
        migrations.RunPython(update_install_template, migrations.RunPython.noop),
    ]