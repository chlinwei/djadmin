from django.db import migrations


def forwards_materialize_hosts(apps, schema_editor):
    """把 Inventory 勾选的分组固化成主机 ID 列表：迁移后往分组里新加主机不会自动进入已有任务/Workflow。"""
    inventory_model = apps.get_model('automation', 'AutomationInventory')
    host_group = apps.get_model('assets', 'HostGroup')
    host = apps.get_model('assets', 'Host')

    child_map = {}
    for group_id, parent_id in host_group.objects.values_list('id', 'parent_id'):
        child_map.setdefault(parent_id, []).append(group_id)

    rows = inventory_model.objects.values_list('id', 'selected_group_ids', 'selected_host_ids')
    for inventory_id, group_ids, host_ids in list(rows):
        roots = [int(item) for item in (group_ids or []) if str(item).isdigit()]
        if not roots:
            continue
        expanded = set()
        pending = list(roots)
        while pending:
            current = pending.pop()
            if current in expanded:
                continue
            expanded.add(current)
            pending.extend(child_map.get(current, []))
        merged = list(dict.fromkeys(
            [int(item) for item in (host_ids or []) if str(item).isdigit()]
            + list(host.objects.filter(group_id__in=expanded, ip__isnull=False).values_list('id', flat=True))
        ))
        inventory_model.objects.filter(pk=inventory_id).update(selected_host_ids=merged)


class Migration(migrations.Migration):

    dependencies = [
        ('automation', '0048_remove_shell_script_automation'),
        # 数据迁移要按 Host.ip 过滤，依赖 assets 初始表结构。
        ('assets', '0001_initial'),
    ]

    operations = [
        # 分组信息固化后不再需要，回滚只恢复空字段（无法还原原始勾选的分组）。
        migrations.RunPython(forwards_materialize_hosts, migrations.RunPython.noop),
        migrations.RemoveField(model_name='automationinventory', name='selected_group_ids'),
    ]
