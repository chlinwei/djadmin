from django.db import migrations


def forwards_materialize_hosts(apps, schema_editor):
    """把勾选分组固化成主机 ID 列表：迁移后往分组里新加主机不会自动进入已有巡检任务。"""
    inspection_task = apps.get_model('inspection', 'InspectionTask')
    host_group = apps.get_model('assets', 'HostGroup')
    host = apps.get_model('assets', 'Host')

    child_map = {}
    for group_id, parent_id in host_group.objects.values_list('id', 'parent_id'):
        child_map.setdefault(parent_id, []).append(group_id)

    rows = inspection_task.objects.values_list('id', 'selected_group_ids', 'selected_host_ids')
    for task_id, group_ids, host_ids in list(rows):
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
            + list(host.objects.filter(group_id__in=expanded, is_deleted_in_cloud=False).values_list('id', flat=True))
        ))
        inspection_task.objects.filter(pk=task_id).update(selected_host_ids=merged)


class Migration(migrations.Migration):

    dependencies = [
        ('inspection', '0009_inspectiontask_host_scope'),
        # 数据迁移要读 Host.is_deleted_in_cloud，必须等该字段所在的 assets 迁移先落地。
        ('assets', '0007_cloudaccount_alter_credential_options_and_more'),
    ]

    operations = [
        # 分组信息在固化后不再需要，回滚只恢复空字段（无法还原原始勾选的分组）。
        migrations.RunPython(forwards_materialize_hosts, migrations.RunPython.noop),
        migrations.RemoveField(model_name='inspectiontask', name='selected_group_ids'),
    ]
