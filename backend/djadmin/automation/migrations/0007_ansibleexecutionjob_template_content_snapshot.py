from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('automation', '0006_ansibleexecutionjob_job_output'),
    ]

    operations = [
        migrations.AddField(
            model_name='ansibleexecutionjob',
            name='template_content_snapshot',
            field=models.TextField(blank=True, default=''),
        ),
    ]
