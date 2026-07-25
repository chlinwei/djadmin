from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('monitor', '0023_alert_rule_group_and_deploy_history'),
    ]

    operations = [
        migrations.AddField(
            model_name='alertrule',
            name='remark',
            field=models.TextField(blank=True, default='', null=True, verbose_name='备注'),
        ),
    ]
