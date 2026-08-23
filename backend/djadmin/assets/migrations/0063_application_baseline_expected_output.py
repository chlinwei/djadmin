from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0062_normalize_shell_work_directory'),
    ]

    operations = [
        migrations.AddField(
            model_name='applicationbaselinecheck',
            name='expected_output',
            field=models.TextField(blank=True, default='', verbose_name='期望输出'),
        ),
    ]