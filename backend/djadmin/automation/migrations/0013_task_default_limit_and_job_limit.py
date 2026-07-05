from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('automation', '0012_inventory_sync_fields'),
    ]

    operations = [
        migrations.AddField(
            model_name='automationtask',
            name='default_limit',
            field=models.CharField(blank=True, default='', max_length=255),
        ),
        migrations.AddField(
            model_name='ansibleexecutionjob',
            name='limit',
            field=models.CharField(blank=True, default='', max_length=255),
        ),
    ]
