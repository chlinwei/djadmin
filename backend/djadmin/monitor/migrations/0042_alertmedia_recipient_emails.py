from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('monitor', '0041_alertroute'),
    ]

    operations = [
        migrations.AddField(
            model_name='alertmedia',
            name='recipient_emails',
            field=models.JSONField(blank=True, default=list),
        ),
    ]
