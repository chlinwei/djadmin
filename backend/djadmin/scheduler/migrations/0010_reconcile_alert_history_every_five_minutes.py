from django.db import migrations


def update_reconcile_schedule(apps, schema_editor):
    ScheduledTask = apps.get_model('scheduler', 'ScheduledTask')
    ScheduledTask.objects.filter(code='reconcile_prometheus_alert_history').update(
        cron_expression='*/5 * * * *',
        next_run_time=None,
    )


def restore_previous_schedule(apps, schema_editor):
    ScheduledTask = apps.get_model('scheduler', 'ScheduledTask')
    ScheduledTask.objects.filter(code='reconcile_prometheus_alert_history').update(
        cron_expression='45 0 * * *',
        next_run_time=None,
    )


class Migration(migrations.Migration):

    dependencies = [
        ('scheduler', '0009_alter_scheduledtask_create_time_and_more'),
    ]

    operations = [
        migrations.RunPython(update_reconcile_schedule, restore_previous_schedule),
    ]
