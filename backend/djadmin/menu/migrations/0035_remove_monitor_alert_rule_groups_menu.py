from django.db import migrations


def remove_monitor_alert_rule_groups_menu(apps, schema_editor):
    # 告警规则改为只读展示 Prometheus 侧规则，规则组是 Prometheus 原生概念（rules.groups[]），
    # 不再需要本地维护规则组，随模型一起下线该菜单，避免留下指向已删除页面的死链接。
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    groups_menu = SysMenu.objects.filter(path='/monitor/alert-rule-groups', menu_type='C').order_by('id').first()
    if groups_menu:
        SysRoleMenu.objects.filter(menu_id=groups_menu.id).delete()
        groups_menu.delete()


class Migration(migrations.Migration):

    dependencies = [
        ('menu', '0034_add_monitor_alert_rule_groups_menu'),
    ]

    operations = [
        migrations.RunPython(remove_monitor_alert_rule_groups_menu, migrations.RunPython.noop),
    ]
