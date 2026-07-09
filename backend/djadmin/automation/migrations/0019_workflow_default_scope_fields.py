from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('automation', '0018_ansibleexecutionjob_become_enabled_snapshot_and_more'),
    ]

    operations = [
        migrations.AddField(
            model_name='automationworkflowtemplate',
            name='default_inventory',
            field=models.ForeignKey(
                blank=True,
                null=True,
                on_delete=models.SET_NULL,
                related_name='workflows',
                to='automation.automationinventory',
            ),
        ),
        migrations.AddField(
            model_name='automationworkflowtemplate',
            name='default_limit',
            field=models.CharField(blank=True, default='', max_length=255),
        ),
    ]
