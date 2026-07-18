from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('user', '0008_agenttoken'),
    ]

    operations = [
        migrations.AddField(
            model_name='agenttoken',
            name='bind_mode',
            field=models.CharField(
                choices=[('shared', '共享'), ('none_shared', '非共享')],
                default='none_shared',
                max_length=32,
                verbose_name='绑定模式',
            ),
        ),
    ]
