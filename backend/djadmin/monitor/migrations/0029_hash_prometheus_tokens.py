from django.contrib.auth.hashers import make_password
from django.db import migrations


PROMETHEUS_HTTP_SD_TOKEN_KEY = 'monitor.prometheus.http_sd_token'
PROMETHEUS_ALERT_WEBHOOK_TOKEN_KEY = 'monitor.prometheus.alert_webhook_token'


def forwards_hash_existing_tokens(apps, schema_editor):
    """一次性把已经以明文存在 SysConfig 里的两个 Prometheus token 就地哈希，
    并把 value_type 切到 'secret'。哈希后的值和原明文校验结果一致，不需要重新
    生成 token / 改动 Prometheus 端已经写好的配置。"""
    SysConfig = apps.get_model('sys_config', 'SysConfig')
    for key in (PROMETHEUS_HTTP_SD_TOKEN_KEY, PROMETHEUS_ALERT_WEBHOOK_TOKEN_KEY):
        cfg = SysConfig.objects.filter(key=key).first()
        if cfg is None:
            continue
        if cfg.value_type == 'secret':
            # 已经是 secret 类型说明已经跑过这次迁移或者是后来才创建的行，不重复哈希。
            continue
        cfg.value = make_password(str(cfg.value or ''))
        cfg.value_type = 'secret'
        cfg.save(update_fields=['value', 'value_type', 'update_time'])


def backwards_noop(apps, schema_editor):
    # 哈希不可逆，无法恢复明文；保留 secret 类型即可，不做任何操作。
    pass


class Migration(migrations.Migration):

    dependencies = [
        ('monitor', '0028_alerthistory'),
        ('sys_config', '0011_add_automation_websocket_poll_interval_configs'),
    ]

    operations = [
        migrations.RunPython(forwards_hash_existing_tokens, backwards_noop),
    ]
