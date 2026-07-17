from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('menu', '0026_merge_playbook_shell_template_menu'),
    ]

    operations = [
        migrations.AddField(
            model_name='sysmenu',
            name='is_expanded',
            field=models.BooleanField(default=True, verbose_name='目录默认展开'),
        ),
    ]
