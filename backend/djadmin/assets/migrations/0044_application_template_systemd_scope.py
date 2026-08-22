from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0043_deployment_templates'),
    ]

    operations = [
        migrations.AddField(
            model_name='applicationdeploymenttemplate',
            name='systemd_scope',
            field=models.CharField(
                choices=[('system', '系统服务'), ('user', '用户服务')],
                default='system',
                max_length=16,
                verbose_name='Systemd 作用域',
            ),
        ),
    ]