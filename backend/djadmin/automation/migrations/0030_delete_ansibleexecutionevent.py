from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('automation', '0029_remove_ansibleexecutionjob_job_output'),
    ]

    operations = [
        migrations.DeleteModel(
            name='AnsibleExecutionEvent',
        ),
    ]
