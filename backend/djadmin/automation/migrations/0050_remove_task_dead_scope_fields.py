from django.db import migrations


class Migration(migrations.Migration):
    """删除 AutomationTask 的执行范围字段：执行范围唯一来源是 Inventory，这两个字段存了但从不参与执行。"""

    dependencies = [
        ('automation', '0049_inventory_fixed_host_scope'),
    ]

    operations = [
        migrations.RemoveField(model_name='automationtask', name='selected_host_ids'),
        migrations.RemoveField(model_name='automationtask', name='selected_group_ids'),
    ]
