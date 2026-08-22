from django.db import migrations, models


def ensure_no_xsd_checks(apps, schema_editor):
    ApplicationBaselineCheck = apps.get_model('assets', 'ApplicationBaselineCheck')
    if ApplicationBaselineCheck.objects.filter(schema_type='xsd').exists():
        raise RuntimeError('存在 XSD 基线检查，请先改写为 Schematron / XPath 后再迁移')


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0056_schema_based_application_baselines'),
    ]

    operations = [
        migrations.RunPython(ensure_no_xsd_checks, migrations.RunPython.noop),
        migrations.AlterField(
            model_name='applicationbaselinecheck',
            name='schema_type',
            field=models.CharField(
                choices=[
                    ('schematron', 'Schematron / XPath'),
                    ('json_schema', 'JSON Schema'),
                ],
                default='schematron',
                max_length=32,
                verbose_name='Schema 类型',
            ),
        ),
    ]