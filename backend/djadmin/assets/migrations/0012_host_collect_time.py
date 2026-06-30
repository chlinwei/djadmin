from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0011_host_port'),
    ]

    operations = [
        migrations.AddField(
            model_name='hosthardware',
            name='collected_at',
            field=models.DateTimeField(blank=True, null=True, verbose_name='最后采集时间'),
        ),
        migrations.AddField(
            model_name='hostsystem',
            name='collected_at',
            field=models.DateTimeField(blank=True, null=True, verbose_name='最后采集时间'),
        ),
    ]
