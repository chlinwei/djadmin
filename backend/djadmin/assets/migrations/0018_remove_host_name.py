from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0017_host_auth_guard_fields'),
    ]

    operations = [
        migrations.RemoveField(
            model_name='host',
            name='name',
        ),
    ]
