from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0078_remove_applicationservice_primary_deployment'),
    ]

    operations = [
        migrations.AlterField(
            model_name='applicationservice',
            name='topology_type',
            field=models.CharField(
                choices=[
                    ('standalone', '单机'),
                    ('cluster', '集群'),
                    ('load_balancer', '负载均衡'),
                ],
                default='standalone',
                max_length=16,
            ),
        ),
    ]
