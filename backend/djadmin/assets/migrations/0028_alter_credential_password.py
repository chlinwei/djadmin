from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0027_remove_host_auth_guard_fields'),
    ]

    operations = [
        migrations.AlterField(
            model_name='credential',
            name='password',
            field=models.CharField(blank=True, max_length=512, null=True),
        ),
    ]
