from django.db import migrations, models


def fill_monitor_target_scrape_port(apps, schema_editor):
    MonitorTarget = apps.get_model('monitor', 'MonitorTarget')
    SoftwarePackage = apps.get_model('monitor', 'SoftwarePackage')

    package_ports = {}
    for pkg in SoftwarePackage.objects.all().order_by('-id'):
        name = str(getattr(pkg, 'name', '') or '').strip()
        if name == '' or name in package_ports:
            continue
        port = getattr(pkg, 'default_port', 9100)
        try:
            port = int(port)
        except (TypeError, ValueError):
            port = 9100
        if port < 1 or port > 65535:
            port = 9100
        package_ports[name] = port

    for target in MonitorTarget.objects.all():
        exporter_type = str(getattr(target, 'exporter_type', '') or '').strip()
        port = package_ports.get(exporter_type, 9100)
        target.scrape_port = port
        target.save(update_fields=['scrape_port'])


def noop_reverse(apps, schema_editor):
    pass


class Migration(migrations.Migration):

    dependencies = [
        ('monitor', '0020_alter_monitortarget_last_dispatch_manual'),
    ]

    operations = [
        migrations.AddField(
            model_name='softwarepackage',
            name='default_port',
            field=models.PositiveIntegerField(default=9100),
        ),
        migrations.AddField(
            model_name='monitortarget',
            name='scrape_port',
            field=models.PositiveIntegerField(default=9100),
        ),
        migrations.RunPython(fill_monitor_target_scrape_port, noop_reverse),
    ]
