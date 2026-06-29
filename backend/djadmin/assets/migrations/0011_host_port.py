# Generated migration for Host.port field

from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0010_host_instance_name'),
    ]

    operations = [
        migrations.AddField(
            model_name='host',
            name='port',
            field=models.PositiveIntegerField(default=22),
        ),
    ]
