from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('automation', '0003_remove_playbooktemplate_enabled'),
    ]

    operations = [
        migrations.RemoveField(
            model_name='playbooktemplate',
            name='default_extra_vars',
        ),
    ]