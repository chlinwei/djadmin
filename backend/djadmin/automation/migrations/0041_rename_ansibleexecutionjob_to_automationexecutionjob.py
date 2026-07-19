from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('automation', '0040_alter_automationexecutiontargetlog_create_time_and_more'),
    ]

    operations = [
        migrations.RenameModel(
            old_name='AnsibleExecutionJob',
            new_name='AutomationExecutionJob',
        ),
    ]
