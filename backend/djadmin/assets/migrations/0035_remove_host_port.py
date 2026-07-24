from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0034_websshsessionlog_effective_username_and_more'),
    ]

    operations = [
        migrations.RemoveField(
            model_name='host',
            name='port',
        ),
    ]
