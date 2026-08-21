from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('monitor', '0047_user_alert_media_binding'),
    ]

    operations = [
        migrations.AddField(
            model_name='alerthistory',
            name='rule_group',
            field=models.CharField(blank=True, default='', max_length=200),
        ),
    ]