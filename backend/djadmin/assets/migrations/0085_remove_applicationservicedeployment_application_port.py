from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0084_application_service_deployment_m2m'),
    ]

    operations = [
        migrations.RemoveField(
            model_name='applicationservicedeployment',
            name='application_port',
        ),
    ]
