from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0014_alter_application_create_time_and_more'),
    ]

    operations = [
        migrations.AddField(
            model_name='websshsessionlog',
            name='input_content',
            field=models.TextField(blank=True, default=''),
        ),
        migrations.AddField(
            model_name='websshsessionlog',
            name='output_content',
            field=models.TextField(blank=True, default=''),
        ),
        migrations.AddField(
            model_name='websshsessionlog',
            name='recorded_content_bytes',
            field=models.IntegerField(default=0),
        ),
        migrations.AddField(
            model_name='websshsessionlog',
            name='is_content_truncated',
            field=models.BooleanField(default=False),
        ),
    ]
