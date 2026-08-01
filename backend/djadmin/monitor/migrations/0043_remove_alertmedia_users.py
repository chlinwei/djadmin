from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('monitor', '0042_alertmedia_recipient_emails'),
        ('user', '0015_remove_sysuser_email'),
    ]

    operations = [
        migrations.RemoveField(
            model_name='alertmedia',
            name='users',
        ),
    ]
