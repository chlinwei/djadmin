from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0035_remove_host_port'),
    ]

    operations = [
        migrations.AddField(
            model_name='host',
            name='webssh_default_username',
            field=models.CharField(blank=True, default='root', max_length=100),
        ),
        migrations.AddField(
            model_name='host',
            name='webssh_login_users',
            field=models.CharField(blank=True, default='root', max_length=512),
        ),
    ]
