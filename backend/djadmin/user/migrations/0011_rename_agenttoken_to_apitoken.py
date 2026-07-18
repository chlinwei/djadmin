from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('user', '0010_backfill_agenttoken_bind_mode'),
    ]

    operations = [
        migrations.RenameModel(
            old_name='AgentToken',
            new_name='ApiToken',
        ),
    ]
