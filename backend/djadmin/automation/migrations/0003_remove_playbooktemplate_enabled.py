from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('automation', '0002_automationtask_ansibleexecutionjob_task'),
    ]

    operations = [
        migrations.RemoveField(
            model_name='playbooktemplate',
            name='enabled',
        ),
    ]
