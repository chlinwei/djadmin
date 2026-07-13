from datetime import date

from django.db import migrations


OLD_TO_NEW_COMPONENTS = {
    'sys/audit/webssh/index': 'audit/webssh/index',
    'sys/audit/login/index': 'audit/login/index',
    'sys/audit/operationLog/index': 'audit/operationLog/index',
}

NEW_TO_OLD_COMPONENTS = {value: key for key, value in OLD_TO_NEW_COMPONENTS.items()}


def _normalize_component(component, mapping):
    value = str(component or '').strip()
    if not value:
        return value
    if value in mapping:
        return mapping[value]
    if value.startswith('sys/audit/') and mapping is OLD_TO_NEW_COMPONENTS:
        return value.replace('sys/audit/', 'audit/', 1)
    if value.startswith('audit/') and mapping is NEW_TO_OLD_COMPONENTS:
        return value.replace('audit/', 'sys/audit/', 1)
    return value


def _update_components(apps, mapping):
    SysMenu = apps.get_model('menu', 'SysMenu')
    today = date.today()

    for menu in SysMenu.objects.all().only('id', 'component', 'update_time'):
        normalized = _normalize_component(menu.component, mapping)
        if normalized == menu.component:
            continue
        menu.component = normalized
        menu.update_time = today
        menu.save(update_fields=['component', 'update_time'])


def forward(apps, schema_editor):
    _update_components(apps, OLD_TO_NEW_COMPONENTS)


def backward(apps, schema_editor):
    _update_components(apps, NEW_TO_OLD_COMPONENTS)


class Migration(migrations.Migration):

    dependencies = [
        ('menu', '0023_restore_workflow_menu'),
    ]

    operations = [
        migrations.RunPython(forward, backward),
    ]
