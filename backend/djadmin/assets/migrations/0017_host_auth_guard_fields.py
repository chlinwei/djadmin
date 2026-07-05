from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0016_remove_websshsessionlog_session_id'),
    ]

    operations = [
        migrations.AddField(
            model_name='host',
            name='auth_failed_count',
            field=models.PositiveIntegerField(default=0, verbose_name='连续认证失败次数'),
        ),
        migrations.AddField(
            model_name='host',
            name='last_auth_failed_time',
            field=models.DateTimeField(blank=True, null=True, verbose_name='最后认证失败时间'),
        ),
        migrations.AddField(
            model_name='host',
            name='auth_lock_until',
            field=models.DateTimeField(blank=True, null=True, verbose_name='认证失败保护截止时间'),
        ),
    ]
