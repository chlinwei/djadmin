from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0064_application_baseline_requires_running'),
    ]

    operations = [
        migrations.AddField(
            model_name='applicationdeployment',
            name='last_status_check_time',
            field=models.DateTimeField(blank=True, null=True),
        ),
        migrations.AddField(
            model_name='applicationdeployment',
            name='runtime_status',
            field=models.CharField(
                choices=[
                    ('unknown', '未知'),
                    ('running', '运行中'),
                    ('stopped', '已停止'),
                    ('error', '状态检查失败'),
                ],
                default='unknown',
                max_length=16,
            ),
        ),
        migrations.AddField(
            model_name='applicationdeployment',
            name='runtime_status_output',
            field=models.TextField(blank=True, default=''),
        ),
    ]