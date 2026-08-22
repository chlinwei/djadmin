from django.db import migrations, models
from django.db.models import Count


def copy_unique_instance_names_to_agent_id(apps, schema_editor):
    Host = apps.get_model('assets', 'Host')
    duplicate_names = set(
        Host.objects.exclude(instance_name__isnull=True)
        .exclude(instance_name='')
        .values('instance_name')
        .annotate(total=Count('id'))
        .filter(total__gt=1)
        .values_list('instance_name', flat=True)
    )
    for host in Host.objects.exclude(instance_name__isnull=True).exclude(instance_name='').iterator():
        if host.instance_name not in duplicate_names:
            host.agent_id = host.instance_name
            host.save(update_fields=['agent_id'])


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0044_application_template_systemd_scope'),
    ]

    operations = [
        migrations.AddField(
            model_name='host',
            name='agent_id',
            field=models.CharField(blank=True, max_length=128, null=True, unique=True, verbose_name='Agent ID'),
        ),
        migrations.RunPython(copy_unique_instance_names_to_agent_id, migrations.RunPython.noop),
    ]