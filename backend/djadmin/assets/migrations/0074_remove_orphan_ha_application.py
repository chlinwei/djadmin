from django.db import migrations


def remove_orphan_ha_application(apps, schema_editor):
    Application = apps.get_model('assets', 'Application')
    application = Application.objects.filter(code='ha').first()
    if application is None:
        return
    has_references = (
        application.versions.exists()
        or application.deployment_templates.exists()
        or application.cluster_profiles.exists()
        or application.services.exists()
    )
    if not has_references:
        application.delete()


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0073_alter_clusterprofile_application'),
    ]

    operations = [
        migrations.RunPython(remove_orphan_ha_application, migrations.RunPython.noop),
    ]