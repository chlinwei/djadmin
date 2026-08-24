from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0080_remove_applicationservice_access_type'),
    ]

    operations = [
        migrations.AddField(
            model_name='applicationdeployment',
            name='ha_role',
            field=models.CharField(
                choices=[('unknown', '未知'), ('primary', '主'), ('standby', '备')],
                default='unknown',
                max_length=16,
            ),
        ),
    ]
