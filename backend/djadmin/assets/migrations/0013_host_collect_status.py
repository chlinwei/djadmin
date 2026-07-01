from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0012_host_collect_time'),
    ]

    operations = [
        migrations.AddField(
            model_name='host',
            name='collect_status',
            field=models.CharField(default='unknown', max_length=16, verbose_name='采集状态'),
        ),
        migrations.AddField(
            model_name='host',
            name='collect_message',
            field=models.TextField(blank=True, default='', verbose_name='采集失败原因'),
        ),
        migrations.AddField(
            model_name='host',
            name='collect_time',
            field=models.DateTimeField(blank=True, null=True, verbose_name='最后采集时间'),
        ),
    ]
