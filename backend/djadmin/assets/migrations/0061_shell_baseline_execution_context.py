from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0060_add_shell_baseline_type'),
    ]

    operations = [
        migrations.AddField(
            model_name='applicationbaselinecheck',
            name='script_executor',
            field=models.CharField(default='${RUN_USER}', max_length=100, verbose_name='脚本执行者'),
        ),
        migrations.AddField(
            model_name='applicationbaselinecheck',
            name='work_directory',
            field=models.CharField(default='${APP_HOME}', max_length=512, verbose_name='运行目录'),
        ),
    ]