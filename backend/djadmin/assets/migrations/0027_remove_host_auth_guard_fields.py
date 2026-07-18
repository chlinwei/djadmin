from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0026_agent_job_event'),
    ]

    operations = [
        migrations.RemoveField(
            model_name='host',
            name='auth_failed_count',
        ),
        migrations.RemoveField(
            model_name='host',
            name='last_auth_failed_time',
        ),
        migrations.RemoveField(
            model_name='host',
            name='auth_lock_until',
        ),
    ]
