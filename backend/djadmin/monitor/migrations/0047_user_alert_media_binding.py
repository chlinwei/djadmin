from django.db import migrations, models
import django.db.models.deletion


class Migration(migrations.Migration):

    dependencies = [
        ('user', '0016_sysuser_alert_media'),
        ('monitor', '0046_remove_monitor_target_install_job'),
    ]

    operations = [
        # 如果 AlertMedia 之前添加了 recipients 字段，这里删除它
        migrations.RemoveField(
            model_name='alertmedia',
            name='recipients',
        ) if False else migrations.RunPython(migrations.RunPython.noop),
        
        # 创建用户媒介绑定表
        migrations.CreateModel(
            name='UserAlertMediaBinding',
            fields=[
                ('id', models.BigAutoField(auto_created=True, primary_key=True, serialize=False)),
                ('create_time', models.DateTimeField(auto_now_add=True)),
                ('update_time', models.DateTimeField(auto_now=True)),
                ('remark', models.CharField(blank=True, default='', max_length=500)),
                ('recipients', models.JSONField(blank=True, default=list)),
                ('enabled', models.BooleanField(default=True)),
                ('media', models.ForeignKey(on_delete=django.db.models.deletion.CASCADE, related_name='user_bindings', to='monitor.alertmedia')),
                ('user', models.ForeignKey(on_delete=django.db.models.deletion.CASCADE, related_name='alert_media_bindings', to='user.sysuser')),
            ],
            options={
                'db_table': 'monitor_user_alert_media_binding',
            },
        ),
        # 添加 unique_together 约束
        migrations.AlterUniqueTogether(
            name='useralertmediabinding',
            unique_together={('user', 'media')},
        ),
    ]
