from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0015_webssh_session_content_fields'),
    ]

    operations = [
        migrations.RemoveField(
            model_name='websshsessionlog',
            name='session_id',
        ),
    ]

