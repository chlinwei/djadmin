from django.db import migrations


def seed_node_exporter_amd64(apps, schema_editor):
    """一次性数据迁移：预置 node_exporter linux-amd64 占位记录（无 arm64，按用户要求仅默认 amd64）。
    仅在部署时执行一次，用户后续手动删除该记录不会被自动重建（区别于早期“每次进页面都调用
    ensure_defaults 接口”的方案——那种方案会在用户删除后又把记录建回来）。"""
    SoftwarePackage = apps.get_model('monitor', 'SoftwarePackage')
    exists = SoftwarePackage.objects.filter(name='node_exporter', os='linux', arch='amd64').exists()
    if not exists:
        SoftwarePackage.objects.create(
            name='node_exporter',
            version='1.8.2',
            os='linux',
            arch='amd64',
            file='',
        )


def noop_reverse(apps, schema_editor):
    # 数据迁移的回滚不做任何删除，避免误删用户已同步的软件包
    pass


class Migration(migrations.Migration):

    dependencies = [
        ('monitor', '0003_alter_softwarepackage_file'),
    ]

    operations = [
        migrations.RunPython(seed_node_exporter_amd64, noop_reverse),
    ]
