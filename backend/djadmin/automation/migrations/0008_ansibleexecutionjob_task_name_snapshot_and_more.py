from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('automation', '0007_ansibleexecutionjob_template_content_snapshot'),
    ]

    operations = [
        migrations.AddField(
            model_name='ansibleexecutionjob',
            name='task_name_snapshot',
            field=models.CharField(blank=True, default='', max_length=128),
        ),
        migrations.AddField(
            model_name='ansibleexecutionjob',
            name='template_name_snapshot',
            field=models.CharField(blank=True, default='', max_length=128),
        ),
    ]
