from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('scheduler', '0007_update_time_to_datetime'),
    ]

    operations = [
        migrations.AddField(
            model_name='scheduledtask',
            name='cron_expression',
            field=models.CharField(blank=True, max_length=120, null=True),
        ),
    ]
