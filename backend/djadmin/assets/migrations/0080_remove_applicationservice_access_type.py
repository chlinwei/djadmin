from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0079_alter_applicationservice_topology_type'),
    ]

    operations = [
        migrations.RemoveField(
            model_name='applicationservice',
            name='access_type',
        ),
    ]
