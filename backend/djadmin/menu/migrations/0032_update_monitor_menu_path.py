from django.db import migrations


def forward_update_monitor_menu_path(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysMenu.objects.filter(path='/sys/monitor', menu_type='C').update(path='/monitor')


def reverse_update_monitor_menu_path(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysMenu.objects.filter(path='/monitor', menu_type='C').update(path='/sys/monitor')


class Migration(migrations.Migration):

    dependencies = [
        ('menu', '0031_add_monitor_menu'),
    ]

    operations = [
        migrations.RunPython(forward_update_monitor_menu_path, reverse_update_monitor_menu_path),
    ]
