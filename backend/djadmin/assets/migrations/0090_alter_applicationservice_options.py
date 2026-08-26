from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0089_global_business_environment'),
    ]

    operations = [
        migrations.AlterModelOptions(
            name='applicationservice',
            options={'ordering': ['business_system_id', 'environment_id', 'name']},
        ),
    ]
