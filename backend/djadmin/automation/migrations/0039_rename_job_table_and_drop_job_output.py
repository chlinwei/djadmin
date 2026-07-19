from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('automation', '0038_ansibleexecutionhostlog'),
    ]

    operations = [
        migrations.AlterModelTable(
            name='ansibleexecutionjob',
            table='automation_execution_job',
        ),
        migrations.RemoveField(
            model_name='ansibleexecutionjob',
            name='job_output',
        ),
    ]
