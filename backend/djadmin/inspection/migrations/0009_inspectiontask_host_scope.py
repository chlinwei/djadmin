from django.db import migrations, models


def forwards_copy_host_group(apps, schema_editor):
    """单选主机组迁移到多选范围：host_group_id -> selected_group_ids=[id]。"""
    inspection_task = apps.get_model('inspection', 'InspectionTask')
    for task_id, host_group_id in inspection_task.objects.exclude(host_group_id=None).values_list('id', 'host_group_id'):
        inspection_task.objects.filter(pk=task_id).update(selected_group_ids=[host_group_id])


def backwards_restore_host_group(apps, schema_editor):
    inspection_task = apps.get_model('inspection', 'InspectionTask')
    for task_id, group_ids in inspection_task.objects.values_list('id', 'selected_group_ids'):
        first_group_id = next((item for item in (group_ids or []) if isinstance(item, int)), None)
        inspection_task.objects.filter(pk=task_id).update(host_group_id=first_group_id)


class Migration(migrations.Migration):

    dependencies = [
        ('inspection', '0008_group_scope_target_type'),
    ]

    operations = [
        migrations.AddField(
            model_name='inspectiontask',
            name='selected_group_ids',
            field=models.JSONField(blank=True, default=list, verbose_name='勾选主机组'),
        ),
        migrations.AddField(
            model_name='inspectiontask',
            name='selected_host_ids',
            field=models.JSONField(blank=True, default=list, verbose_name='勾选主机'),
        ),
        migrations.RunPython(forwards_copy_host_group, backwards_restore_host_group),
        migrations.RemoveField(model_name='inspectiontask', name='host_group'),
    ]
