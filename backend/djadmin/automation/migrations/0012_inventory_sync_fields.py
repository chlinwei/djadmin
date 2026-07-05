from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('automation', '0011_automationtask_inventory_fk'),
    ]

    operations = [
        migrations.AddField(
            model_name='automationinventory',
            name='last_sync_host_count',
            field=models.PositiveIntegerField(default=0),
        ),
        migrations.AddField(
            model_name='automationinventory',
            name='last_sync_message',
            field=models.TextField(blank=True, default=''),
        ),
        migrations.AddField(
            model_name='automationinventory',
            name='last_sync_status',
            field=models.CharField(choices=[('never', 'Never'), ('success', 'Success'), ('failed', 'Failed')], default='never', max_length=16),
        ),
        migrations.AddField(
            model_name='automationinventory',
            name='last_sync_time',
            field=models.DateTimeField(blank=True, null=True),
        ),
        migrations.AddField(
            model_name='automationinventory',
            name='update_cache_timeout',
            field=models.PositiveIntegerField(default=300),
        ),
        migrations.AddField(
            model_name='automationinventory',
            name='update_on_launch',
            field=models.BooleanField(default=False),
        ),
    ]
