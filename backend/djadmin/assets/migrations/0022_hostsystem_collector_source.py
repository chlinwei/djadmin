from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0021_alter_application_create_time_and_more'),
    ]

    operations = [
        migrations.AddField(
            model_name='hostsystem',
            name='collector_source',
            field=models.CharField(blank=True, max_length=32, null=True),
        ),
    ]
