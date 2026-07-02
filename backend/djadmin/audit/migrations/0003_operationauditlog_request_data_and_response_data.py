from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('audit', '0002_operationauditlog'),
    ]

    operations = [
        migrations.AddField(
            model_name='operationauditlog',
            name='request_data',
            field=models.TextField(blank=True, default=''),
        ),
        migrations.AddField(
            model_name='operationauditlog',
            name='response_data',
            field=models.TextField(blank=True, default=''),
        ),
    ]