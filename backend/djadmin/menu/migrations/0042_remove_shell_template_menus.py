from django.db import migrations


SHELL_TEMPLATE_PERMS = {
    'automation:shell_scripts:view',
    'automation:shell_scripts:create',
    'automation:shell_scripts:update',
    'automation:shell_scripts:delete',
}


def remove_shell_template_menus(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    shell_menu_ids = list(SysMenu.objects.filter(perms__in=SHELL_TEMPLATE_PERMS).values_list('id', flat=True))
    if shell_menu_ids:
        SysRoleMenu.objects.filter(menu_id__in=shell_menu_ids).delete()
        SysMenu.objects.filter(id__in=shell_menu_ids).delete()


class Migration(migrations.Migration):

    dependencies = [
        ('menu', '0041_add_monitor_alert_route_menu'),
    ]

    operations = [
        migrations.RunPython(remove_shell_template_menus, migrations.RunPython.noop),
    ]