from django.db import migrations


# 说明：本迁移原先会删除 Prometheus http_sd / alert_webhook 的 token 配置，
# 但这两个 token 仍被 push(v2)/http-sd 链路使用，删除属于误操作，已中和为 no-op。
# 保留文件名与依赖链，避免破坏已 apply 环境的迁移历史；全新部署走 no-op 不再删 token。


class Migration(migrations.Migration):

    dependencies = [
        ('monitor', '0029_hash_prometheus_tokens'),
    ]

    operations = []
