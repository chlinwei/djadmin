from django.db import migrations, models


def normalize_shell_work_directory(apps, schema_editor):
    ApplicationBaselineCheck = apps.get_model('assets', 'ApplicationBaselineCheck')
    ApplicationBaselineCheck.objects.filter(
        schema_type='shell',
        work_directory='',
    ).update(work_directory='${APP_HOME}')


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0061_shell_baseline_execution_context'),
    ]

    operations = [
        migrations.AlterField(
            model_name='applicationbaselinecheck',
            name='work_directory',
            field=models.CharField(default='${APP_HOME}', max_length=512, verbose_name='运行目录'),
        ),
        migrations.RunPython(normalize_shell_work_directory, migrations.RunPython.noop),
    ]