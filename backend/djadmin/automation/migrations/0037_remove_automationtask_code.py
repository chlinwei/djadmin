from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('automation', '0036_remove_shellscripttemplate_parameters'),
    ]

    operations = [
        migrations.RemoveField(
            model_name='automationtask',
            name='code',
        ),
    ]
