from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0036_host_webssh_user_preferences'),
    ]

    operations = [
        migrations.AddField(
            model_name='hostsystem',
            name='timezone_name',
            field=models.CharField(blank=True, max_length=64, null=True),
        ),
        migrations.AddField(
            model_name='hostsystem',
            name='utc_offset',
            field=models.CharField(blank=True, max_length=16, null=True),
        ),
    ]