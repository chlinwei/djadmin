from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('automation', '0030_delete_ansibleexecutionevent'),
    ]

    operations = [
        migrations.AddField(
            model_name='ansibleexecutionjob',
            name='job_output',
            field=models.TextField(blank=True, default=''),
        ),
    ]
