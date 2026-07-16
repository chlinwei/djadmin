from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('automation', '0031_ansibleexecutionjob_job_output'),
    ]

    operations = [
        migrations.DeleteModel(
            name='AnsibleExecutionTarget',
        ),
    ]
