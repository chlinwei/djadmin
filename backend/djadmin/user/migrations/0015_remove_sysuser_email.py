from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('user', '0014_alter_apitoken_bind_mode_alter_apitoken_created_by'),
    ]

    operations = [
        migrations.RemoveField(
            model_name='sysuser',
            name='email',
        ),
    ]
