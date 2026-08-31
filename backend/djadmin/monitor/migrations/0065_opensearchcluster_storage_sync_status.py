from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('monitor', '0064_logcollectionfilterrule'),
    ]

    operations = [
        migrations.AddField(
            model_name='opensearchcluster',
            name='storage_sync_error',
            field=models.TextField(blank=True, default='', verbose_name='存储配置同步错误'),
        ),
        migrations.AddField(
            model_name='opensearchcluster',
            name='storage_sync_status',
            field=models.CharField(
                choices=[
                    ('unknown', '未同步'), ('pending', '同步中'),
                    ('success', '已同步'), ('failed', '同步失败'),
                ],
                default='unknown', max_length=16, verbose_name='存储配置同步状态',
            ),
        ),
        migrations.AddField(
            model_name='opensearchcluster',
            name='storage_sync_time',
            field=models.DateTimeField(blank=True, null=True, verbose_name='最近存储配置同步时间'),
        ),
    ]