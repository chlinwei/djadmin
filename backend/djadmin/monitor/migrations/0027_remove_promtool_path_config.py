# 数据迁移：删除遗留的 monitor.prometheus.promtool_path 配置项。
# 该 key 用于旧版"本地导出 YAML + promtool 校验 + 部署"流程，0026 迁移清理配置时
# 遗漏了这个 key（当时清理列表用的是 sys.monitor.prometheus.* 命名），现单独补删。
from django.db import migrations


def remove_promtool_path_config(apps, schema_editor):
    SysConfig = apps.get_model('sys_config', 'SysConfig')
    SysConfig.objects.filter(key='monitor.prometheus.promtool_path').delete()


class Migration(migrations.Migration):

    dependencies = [
        ('monitor', '0026_remove_alert_rule_models'),
    ]

    operations = [
        migrations.RunPython(remove_promtool_path_config, migrations.RunPython.noop),
    ]
