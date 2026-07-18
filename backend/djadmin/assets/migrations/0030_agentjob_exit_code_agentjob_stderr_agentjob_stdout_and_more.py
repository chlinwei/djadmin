from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0029_rename_assets_agen_agent_i_2583a5_idx_assets_agen_agent_i_ed2f13_idx_and_more'),
    ]

    operations = [
        migrations.AddField(
            model_name='agentjob',
            name='exit_code',
            field=models.IntegerField(default=-1, verbose_name='退出码'),
        ),
        migrations.AddField(
            model_name='agentjob',
            name='stderr',
            field=models.TextField(blank=True, default='', verbose_name='标准错误'),
        ),
        migrations.AddField(
            model_name='agentjob',
            name='stdout',
            field=models.TextField(blank=True, default='', verbose_name='标准输出'),
        ),
        migrations.AddField(
            model_name='host',
            name='agent_online',
            field=models.BooleanField(default=False, verbose_name='Agent在线状态'),
        ),
        migrations.AddField(
            model_name='host',
            name='host_snapshot',
            field=models.JSONField(blank=True, default=dict, verbose_name='主机快照数据'),
        ),
    ]
