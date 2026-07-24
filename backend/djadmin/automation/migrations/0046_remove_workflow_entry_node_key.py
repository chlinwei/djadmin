from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('automation', '0045_add_template_category'),
    ]

    operations = [
        migrations.RemoveField(
            model_name='automationworkflowtemplate',
            name='entry_node_key',
        ),
    ]
