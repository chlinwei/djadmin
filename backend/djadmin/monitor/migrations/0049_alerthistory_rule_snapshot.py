from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('monitor', '0048_alerthistory_rule_group'),
    ]

    operations = [
        migrations.AddField(
            model_name='alerthistory',
            name='rule_snapshot',
            field=models.JSONField(blank=True, default=dict),
        ),
    ]