from django.db import migrations, models


class Migration(migrations.Migration):
    dependencies = [
        ('inspection', '0004_agent_service_once_scope'),
    ]

    operations = [
        migrations.AlterField(
            model_name='inspectioncheck',
            name='executor',
            field=models.CharField(
                choices=[
                    ('shell', 'Agent Shell'),
                    ('schema_validate', 'Agent Schema'),
                    ('http', 'Agent HTTP'),
                    ('tcp', 'Agent TCP'),
                ],
                max_length=16,
                verbose_name='执行器',
            ),
        ),
    ]