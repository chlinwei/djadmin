from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('sys_config', '0001_initial'),
    ]

    operations = [
        migrations.AlterField(
            model_name='sysconfig',
            name='update_time',
            field=models.DateTimeField(auto_now=True, verbose_name='修改时间'),
        ),
    ]
