from django.apps import AppConfig


class MonitorConfig(AppConfig):
    default_auto_field = 'django.db.models.BigAutoField'
    name = 'monitor'

    def ready(self):
        # 索引模板由 STANDARD_LOG_FIELDS 生成，属于代码派生物：迁移后自动对齐，
        # 避免升级了代码却仍用旧 mapping（曾导致 error_fingerprint 被动态映射成 text）。
        from django.db.models.signals import post_migrate
        post_migrate.connect(sync_log_storage_after_migrate, sender=self)


def sync_log_storage_after_migrate(sender, **kwargs):
    from .log_management import enqueue_log_storage_sync
    from .models import OpenSearchCluster

    for cluster_id in OpenSearchCluster.objects.filter(enabled=True).values_list('pk', flat=True):
        enqueue_log_storage_sync(cluster_id)
