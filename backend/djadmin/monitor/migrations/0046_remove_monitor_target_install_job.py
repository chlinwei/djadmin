from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('monitor', '0045_history_without_automation_job'),
    ]

    operations = [
        migrations.RemoveField(
            model_name='monitortarget',
            name='last_install_job_id',
        ),
    ]