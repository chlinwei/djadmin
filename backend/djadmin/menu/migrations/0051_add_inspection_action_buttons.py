from datetime import date

from django.db import migrations


# 巡检可以下发 shell 命令到全网主机，必须按动作拆分权限，不能只靠一个页面级 inspection:view。
BUTTON_DEFS = [
    ('inspection:groups:create', '巡检组新增', 1),
    ('inspection:groups:update', '巡检组编辑', 2),
    ('inspection:groups:delete', '巡检组删除', 3),
    ('inspection:tasks:create', '巡检任务新增', 4),
    ('inspection:tasks:update', '巡检任务编辑', 5),
    ('inspection:tasks:delete', '巡检任务删除', 6),
    ('inspection:tasks:run', '巡检任务执行', 7),
    ('inspection:executions:cancel', '巡检执行取消', 8),
]


def forward(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRole = apps.get_model('role', 'SysRole')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    today = date.today()
    page = SysMenu.objects.filter(path='/sys/inspection', menu_type='C').order_by('id').first()
    if page is None:
        return

    admin_role = (
        SysRole.objects.filter(code='admin').order_by('id').first()
        or SysRole.objects.filter(name='超级管理员').order_by('id').first()
    )

    for perms, name, order_num in BUTTON_DEFS:
        button = SysMenu.objects.filter(perms=perms, menu_type='F').order_by('id').first()
        if button is None:
            button = SysMenu.objects.create(
                name=name,
                icon='',
                parent_id=page.id,
                order_num=order_num,
                path='',
                component='',
                menu_type='F',
                perms=perms,
                location=1,
                create_time=today,
                update_time=today,
                remark='inspection action permission',
            )
        if admin_role:
            SysRoleMenu.objects.get_or_create(role_id=admin_role.id, menu_id=button.id)


def backward(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysRoleMenu = apps.get_model('menu', 'SysRoleMenu')

    button_ids = list(
        SysMenu.objects.filter(perms__in=[item[0] for item in BUTTON_DEFS], menu_type='F')
        .values_list('id', flat=True)
    )
    SysRoleMenu.objects.filter(menu_id__in=button_ids).delete()
    SysMenu.objects.filter(id__in=button_ids).delete()


class Migration(migrations.Migration):

    dependencies = [
        ('role', '0001_initial'),
        ('menu', '0050_add_log_retention_menu'),
    ]

    operations = [
        migrations.RunPython(forward, backward),
    ]
