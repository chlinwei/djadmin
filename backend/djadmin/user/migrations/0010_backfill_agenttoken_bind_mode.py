from django.db import migrations


def _backfill_bind_mode(apps, schema_editor):
    AgentToken = apps.get_model('user', 'AgentToken')
    AgentToken.objects.filter(agent_id='global').update(bind_mode='shared')


def _rollback_bind_mode(apps, schema_editor):
    AgentToken = apps.get_model('user', 'AgentToken')
    AgentToken.objects.filter(agent_id='global').update(bind_mode='none_shared')


class Migration(migrations.Migration):

    dependencies = [
        ('user', '0009_agenttoken_bind_mode'),
    ]

    operations = [
        migrations.RunPython(_backfill_bind_mode, _rollback_bind_mode),
    ]
