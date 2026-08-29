# 数据迁移：修正 SysMenu 中不存在于 Font Awesome Free Solid 图标集的图标名。
# 这些值要么是 Ant Design 的图标名（safety-certificate），要么是被误填成中文菜单名，
# 前端 <FontAwesomeIcon :icon="menu.icon"> 解析失败后会在控制台刷
# "Could not find one or more icon(s)" 并渲染成空白占位。
from django.db import migrations

# path -> (旧图标, 新图标)
ICON_FIXES = {
    '/sys/apiToken': ('safety-certificate', 'certificate'),
    '/sys/usercenter/index': ('个人中心', 'user'),
    '/bsns/post': ('post', 'briefcase'),
    '/dev/dev': ('system', 'code'),
    '业务管理11': ('业务管理11', 'folder'),
    '/sys/test': ('test1', 'flask'),
}


def fix_invalid_menu_icons(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    for path, (old_icon, new_icon) in ICON_FIXES.items():
        SysMenu.objects.filter(path=path, icon=old_icon).update(icon=new_icon)


def reverse_fix_invalid_menu_icons(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    for path, (old_icon, new_icon) in ICON_FIXES.items():
        SysMenu.objects.filter(path=path, icon=new_icon).update(icon=old_icon)


class Migration(migrations.Migration):

    dependencies = [
        ('menu', '0051_add_inspection_action_buttons'),
    ]

    operations = [
        migrations.RunPython(fix_invalid_menu_icons, reverse_fix_invalid_menu_icons),
    ]
