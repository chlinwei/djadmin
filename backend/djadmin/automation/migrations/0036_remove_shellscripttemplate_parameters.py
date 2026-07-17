from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('automation', '0035_task_shell_parameters_and_job_shell_snapshots'),
    ]

    operations = [
        migrations.RemoveField(
            model_name='shellscripttemplate',
            name='parameters',
        ),
    ]
