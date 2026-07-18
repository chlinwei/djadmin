from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0024_agentjob'),
    ]

    operations = [
        migrations.AddField(
            model_name='agentjob',
            name='client_request_id',
            field=models.CharField(blank=True, max_length=128, null=True, unique=True),
        ),
    ]
