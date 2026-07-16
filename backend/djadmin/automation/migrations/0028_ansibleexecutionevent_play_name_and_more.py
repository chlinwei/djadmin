from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('automation', '0027_remove_ansibleexecutiontarget_stderr_and_more'),
    ]

    operations = [
        migrations.AddField(
            model_name='ansibleexecutionevent',
            name='play_name',
            field=models.CharField(blank=True, default='', max_length=255),
        ),
        migrations.AddField(
            model_name='ansibleexecutionevent',
            name='task_name',
            field=models.CharField(blank=True, default='', max_length=255),
        ),
    ]
