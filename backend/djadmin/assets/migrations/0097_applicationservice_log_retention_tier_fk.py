import django.db.models.deletion
from django.db import migrations, models


def link_tiers(apps, schema_editor):
    """把原来的字符串档位（hot/std/cold）映射到 monitor.LogRetentionTier 记录。"""
    ApplicationService = apps.get_model('assets', 'ApplicationService')
    LogRetentionTier = apps.get_model('monitor', 'LogRetentionTier')
    tier_by_code = {tier.code: tier.id for tier in LogRetentionTier.objects.all()}
    fallback = tier_by_code.get('std')
    for service in ApplicationService.objects.exclude(legacy_log_retention_tier=''):
        tier_id = tier_by_code.get(service.legacy_log_retention_tier, fallback)
        if tier_id is not None:
            ApplicationService.objects.filter(pk=service.pk).update(log_retention_tier_id=tier_id)


def unlink_tiers(apps, schema_editor):
    ApplicationService = apps.get_model('assets', 'ApplicationService')
    for service in ApplicationService.objects.select_related('log_retention_tier'):
        code = service.log_retention_tier.code if service.log_retention_tier_id else 'std'
        ApplicationService.objects.filter(pk=service.pk).update(legacy_log_retention_tier=code)


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0096_remove_applicationlogdefinition_ingest_pipeline_and_more'),
        ('monitor', '0062_seed_default_retention_tiers'),
    ]

    operations = [
        # 先改名保住旧值，建好外键并搬迁数据后再删除，避免 CharField 直接转 FK 丢配置。
        migrations.RenameField(
            model_name='applicationservice',
            old_name='log_retention_tier',
            new_name='legacy_log_retention_tier',
        ),
        migrations.AddField(
            model_name='applicationservice',
            name='log_retention_tier',
            field=models.ForeignKey(
                blank=True, null=True, on_delete=django.db.models.deletion.PROTECT,
                related_name='services', to='monitor.logretentiontier', verbose_name='日志保留档位',
            ),
        ),
        migrations.RunPython(link_tiers, unlink_tiers),
        migrations.RemoveField(
            model_name='applicationservice',
            name='legacy_log_retention_tier',
        ),
    ]
