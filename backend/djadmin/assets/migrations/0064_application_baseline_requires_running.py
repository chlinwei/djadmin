from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0063_application_baseline_expected_output'),
    ]

    operations = [
        migrations.AddField(
            model_name='applicationbaselinecheck',
            name='requires_running',
            field=models.BooleanField(default=False, verbose_name='仅应用运行时检查'),
        ),
    ]