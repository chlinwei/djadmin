# 数据迁移：将"告警规则"菜单图标从 triangle-exclamation 改为 list-check。
# 背景：triangle-exclamation 语义上更贴近"实时告警"，而该菜单实际展示的是规则定义（只读展示
# Prometheus 侧生效的规则），后续会新增单独的"告警"菜单使用 triangle-exclamation，
# 故这里改用更贴近"规则/清单"语义的 list-check，避免两个菜单图标语义冲突。
from django.db import migrations

OLD_ICON = 'triangle-exclamation'
NEW_ICON = 'list-check'


def fix_alert_rules_menu_icon(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysMenu.objects.filter(path='/monitor/alert-rules', menu_type='C', icon=OLD_ICON).update(icon=NEW_ICON)


def reverse_fix_alert_rules_menu_icon(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysMenu.objects.filter(path='/monitor/alert-rules', menu_type='C', icon=NEW_ICON).update(icon=OLD_ICON)


class Migration(migrations.Migration):

    dependencies = [
        ('menu', '0036_fix_alert_rules_menu_icon'),
    ]

    operations = [
        migrations.RunPython(fix_alert_rules_menu_icon, reverse_fix_alert_rules_menu_icon),
    ]
