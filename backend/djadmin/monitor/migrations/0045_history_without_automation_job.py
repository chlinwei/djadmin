from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('monitor', '0044_remove_alertmedia_recipient_emails'),
    ]

    operations = [
        migrations.RemoveIndex(
            model_name='monitortargetinstallhistory',
            name='monitor_hist_auto_job_id_idx',
        ),
        migrations.RemoveField(
            model_name='monitortargetinstallhistory',
            name='automation_job',
        ),
        migrations.RemoveField(
            model_name='monitortargetinstallhistory',
            name='automation_job_id_snapshot',
        ),
        migrations.RemoveField(
            model_name='monitortargetinstallhistory',
            name='automation_job_uuid_snapshot',
        ),
    ]