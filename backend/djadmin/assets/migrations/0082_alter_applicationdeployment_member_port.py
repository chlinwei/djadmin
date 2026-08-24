from django.db import migrations, models
import django.core.validators


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0081_applicationdeployment_ha_role'),
    ]

    operations = [
        migrations.AlterField(
            model_name='applicationdeployment',
            name='member_port',
            field=models.PositiveIntegerField(
                blank=True,
                null=True,
                validators=[
                    django.core.validators.MinValueValidator(1),
                    django.core.validators.MaxValueValidator(65535),
                ],
                verbose_name='应用端口',
            ),
        ),
    ]
