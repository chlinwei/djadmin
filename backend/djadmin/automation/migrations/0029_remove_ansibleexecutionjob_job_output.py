from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('automation', '0028_ansibleexecutionevent_play_name_and_more'),
    ]

    operations = [
        migrations.RemoveField(
            model_name='ansibleexecutionjob',
            name='job_output',
        ),
    ]
