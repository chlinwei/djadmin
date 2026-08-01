from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('monitor', '0043_remove_alertmedia_users'),
        ('user', '0015_remove_sysuser_email'),
    ]

    operations = [
        migrations.AddField(
            model_name='sysuser',
            name='alert_media',
            field=models.ManyToManyField(blank=True, related_name='bound_users', to='monitor.alertmedia'),
        ),
    ]
