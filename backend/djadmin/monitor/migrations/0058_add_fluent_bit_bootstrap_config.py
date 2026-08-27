from importlib import import_module

from django.db import migrations


def update_install_template(apps, schema_editor):
    playbook_module = import_module('monitor.migrations.0056_bind_fluent_bit_playbooks')
    PlaybookTemplate = apps.get_model('automation', 'PlaybookTemplate')
    PlaybookTemplate.objects.filter(
        name=playbook_module.INSTALL_TEMPLATE_NAME,
    ).update(content=playbook_module.INSTALL_PLAYBOOK)


class Migration(migrations.Migration):

    dependencies = [
        ('monitor', '0057_update_fluent_bit_directory_install'),
    ]

    operations = [
        migrations.RunPython(update_install_template, migrations.RunPython.noop),
    ]