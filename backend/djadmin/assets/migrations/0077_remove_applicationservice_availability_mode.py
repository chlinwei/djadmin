from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0076_business_environment'),
    ]

    operations = [
        migrations.RemoveField(
            model_name='applicationservice',
            name='availability_mode',
        ),
    ]
