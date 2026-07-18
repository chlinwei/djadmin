from datetime import date

from django.db import migrations


PERM_MAPPING = {
    'system:agent_token:view': 'system:api_token:view',
    'system:agent_token:create': 'system:api_token:create',
    'system:agent_token:rotate': 'system:api_token:rotate',
    'system:agent_token:disable': 'system:api_token:disable',
}


def forward(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')

    today = date.today()

    page = (
        SysMenu.objects.filter(perms='system:agent_token:view', menu_type='C').order_by('id').first()
        or SysMenu.objects.filter(path='/sys/agentToken', menu_type='C').order_by('id').first()
        or SysMenu.objects.filter(name='Agent Token管理', menu_type='C').order_by('id').first()
    )
    if page:
        page.name = 'Api Token管理'
        page.path = '/sys/apiToken'
        page.component = 'sys/apiToken/index'
        page.perms = 'system:api_token:view'
        page.update_time = today
        page.save(update_fields=['name', 'path', 'component', 'perms', 'update_time'])

    for old_perm, new_perm in PERM_MAPPING.items():
        if old_perm == 'system:agent_token:view':
            continue
        button = SysMenu.objects.filter(perms=old_perm, menu_type='F').order_by('id').first()
        if button:
            button.perms = new_perm
            button.update_time = today
            button.save(update_fields=['perms', 'update_time'])


def backward(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')

    today = date.today()

    page = (
        SysMenu.objects.filter(perms='system:api_token:view', menu_type='C').order_by('id').first()
        or SysMenu.objects.filter(path='/sys/apiToken', menu_type='C').order_by('id').first()
        or SysMenu.objects.filter(name='Api Token管理', menu_type='C').order_by('id').first()
    )
    if page:
        page.name = 'Agent Token管理'
        page.path = '/sys/agentToken'
        page.component = 'sys/agentToken/index'
        page.perms = 'system:agent_token:view'
        page.update_time = today
        page.save(update_fields=['name', 'path', 'component', 'perms', 'update_time'])

    reverse_mapping = {v: k for k, v in PERM_MAPPING.items()}
    for old_perm, new_perm in reverse_mapping.items():
        if old_perm == 'system:api_token:view':
            continue
        button = SysMenu.objects.filter(perms=old_perm, menu_type='F').order_by('id').first()
        if button:
            button.perms = new_perm
            button.update_time = today
            button.save(update_fields=['perms', 'update_time'])


class Migration(migrations.Migration):

    dependencies = [
        ('menu', '0028_add_agent_token_menu'),
    ]

    operations = [
        migrations.RunPython(forward, backward),
    ]
