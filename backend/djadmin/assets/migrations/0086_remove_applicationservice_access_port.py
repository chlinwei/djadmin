from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0085_remove_applicationservicedeployment_application_port'),
    ]

    operations = [
        migrations.RemoveField(
            model_name='applicationservice',
            name='access_port',
        ),
    ]
