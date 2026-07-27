from django.db import migrations


# http_sd / alert-webhook 已统一走全局 ApiToken 认证（与 dj-agent 同一套），
# 独立的 monitor.prometheus.*_token 参数不再使用，删除避免残留误导。
OBSOLETE_TOKEN_KEYS = (
    'monitor.prometheus.http_sd_token',
    'sys.monitor.prometheus.http_sd_token',
    'monitor.prometheus.alert_webhook_token',
)


def remove_obsolete_prometheus_tokens(apps, schema_editor):
    SysConfig = apps.get_model('sys_config', 'SysConfig')
    SysConfig.objects.filter(key__in=OBSOLETE_TOKEN_KEYS).delete()


def noop_reverse(apps, schema_editor):
    # 参数已统一到 ApiToken，回滚时不重建，避免误恢复无效配置。
    pass


class Migration(migrations.Migration):

    dependencies = [
        ('monitor', '0030_remove_unused_prometheus_tokens'),
    ]

    operations = [
        migrations.RunPython(remove_obsolete_prometheus_tokens, noop_reverse),
    ]
