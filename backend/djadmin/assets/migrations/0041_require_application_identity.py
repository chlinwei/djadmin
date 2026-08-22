from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0040_application_inventory_models'),
    ]

    operations = [
        migrations.AlterField(
            model_name='application',
            name='code',
            field=models.CharField(max_length=64, unique=True, verbose_name='应用编码'),
        ),
        migrations.AlterField(
            model_name='application',
            name='name',
            field=models.CharField(max_length=128, unique=True, verbose_name='应用名称'),
        ),
    ]
