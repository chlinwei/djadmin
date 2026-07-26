# 数据迁移：修复"告警规则"菜单的图标。
# 原值 icon='alert' 不是 Font Awesome Free Solid 图标集中的合法图标名称（该集合里没有 alert），
# 导致前端 FontAwesomeIcon 组件无法正确渲染图标；改为该图标集中已有、语义匹配的 triangle-exclamation。
from django.db import migrations

OLD_ICON = 'alert'
NEW_ICON = 'triangle-exclamation'


def fix_alert_rules_menu_icon(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysMenu.objects.filter(path='/monitor/alert-rules', menu_type='C', icon=OLD_ICON).update(icon=NEW_ICON)


def reverse_fix_alert_rules_menu_icon(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysMenu.objects.filter(path='/monitor/alert-rules', menu_type='C', icon=NEW_ICON).update(icon=OLD_ICON)


class Migration(migrations.Migration):

    dependencies = [
        ('menu', '0035_remove_monitor_alert_rule_groups_menu'),
    ]

    operations = [
        migrations.RunPython(fix_alert_rules_menu_icon, reverse_fix_alert_rules_menu_icon),
    ]
