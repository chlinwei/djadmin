from django.db import migrations, models


class Migration(migrations.Migration):
    dependencies = [
        ('inspection', '0002_inspectiontargetexecution_host_and_more'),
    ]

    operations = [
        migrations.AlterField(
            model_name='inspectionexecution',
            name='status',
            field=models.CharField(
                choices=[
                    ('pending', '等待中'),
                    ('running', '执行中'),
                    ('success', '成功'),
                    ('failed', '失败'),
                    ('canceled', '已取消'),
                ],
                default='pending',
                max_length=16,
            ),
        ),
        migrations.AlterField(
            model_name='inspectiontargetexecution',
            name='status',
            field=models.CharField(
                choices=[
                    ('pending', '等待中'),
                    ('running', '执行中'),
                    ('success', '成功'),
                    ('failed', '失败'),
                    ('canceled', '已取消'),
                ],
                default='pending',
                max_length=16,
            ),
        ),
    ]
