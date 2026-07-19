from django.db import migrations, models
import django.db.models.deletion


class Migration(migrations.Migration):

    dependencies = [
        ('automation', '0041_rename_ansibleexecutionjob_to_automationexecutionjob'),
    ]

    operations = [
        migrations.AlterField(
            model_name='automationexecutiontargetlog',
            name='job',
            field=models.ForeignKey(on_delete=django.db.models.deletion.CASCADE, related_name='target_logs', to='automation.automationexecutionjob'),
        ),
    ]
