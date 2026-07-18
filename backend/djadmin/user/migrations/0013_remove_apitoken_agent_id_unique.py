from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('user', '0012_update_apitoken_bind_mode_values'),
    ]

    operations = [
        migrations.AlterField(
            model_name='apitoken',
            name='agent_id',
            field=models.CharField(max_length=128, verbose_name='Api绑定标识'),
        ),
    ]
