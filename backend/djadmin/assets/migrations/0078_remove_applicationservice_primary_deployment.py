from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0077_remove_applicationservice_availability_mode'),
    ]

    operations = [
        migrations.RemoveField(
            model_name='applicationservice',
            name='primary_deployment',
        ),
    ]
