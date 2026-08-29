from django.db import migrations, models


def move_target_type_into_scope(apps, schema_editor):
    """主机组任务的目标类型并入巡检组范围；必须在删除 target_type 之前执行。"""
    inspection_group = apps.get_model('inspection', 'InspectionGroup')
    host_group_ids = set(
        apps.get_model('inspection', 'InspectionTask').objects
        .filter(target_type='host_group')
        .values_list('group_id', flat=True)
    )
    if host_group_ids:
        inspection_group.objects.filter(pk__in=host_group_ids).update(scope='per_host')


def restore_target_type(apps, schema_editor):
    apps.get_model('inspection', 'InspectionTask').objects.filter(
        group__scope='per_host',
    ).update(target_type='host_group')


class Migration(migrations.Migration):

    dependencies = [
        ('inspection', '0007_inspectiontask_inspection_name'),
    ]

    operations = [
        migrations.AlterField(
            model_name='inspectiongroup',
            name='scope',
            field=models.CharField(
                choices=[
                    ('per_deployment', '逻辑服务·每个部署实例'),
                    ('service_once', '逻辑服务·服务单次'),
                    ('per_host', '主机组·每台主机'),
                ],
                max_length=24,
                verbose_name='执行范围',
            ),
        ),
        migrations.RunPython(move_target_type_into_scope, restore_target_type),
        migrations.RemoveField(
            model_name='inspectiontask',
            name='target_type',
        ),
    ]
