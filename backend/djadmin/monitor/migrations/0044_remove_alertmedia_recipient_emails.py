from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('monitor', '0043_remove_alertmedia_users'),
    ]

    operations = [
        migrations.RemoveField(
            model_name='alertmedia',
            name='recipient_emails',
        ),
    ]