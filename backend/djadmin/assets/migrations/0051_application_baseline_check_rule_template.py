from django.db import migrations, models


def bind_existing_rule_templates(apps, schema_editor):
    ApplicationBaselineCheck = apps.get_model('assets', 'ApplicationBaselineCheck')

    for check in ApplicationBaselineCheck.objects.all().iterator():
        rule = check.rule if isinstance(check.rule, dict) else {}
        attributes = rule.get('attributes') if isinstance(rule.get('attributes'), dict) else {}
        conditions = [condition for condition in attributes.values() if isinstance(condition, dict)]

        if rule.get('assertion') == 'absent':
            template = 'forbidden_xml_element'
        elif any(condition.get('operator') == 'gte_number' for condition in conditions):
            template = 'xml_numeric_min'
        elif any(condition.get('sensitive') or condition.get('operator') == 'csv_contains_all' for condition in conditions):
            template = 'xml_sensitive_list'
        else:
            template = 'attribute_contains_text'

        check.rule_template = template
        check.save(update_fields=['rule_template'])


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0050_generic_application_baseline_checks'),
    ]

    operations = [
        migrations.AddField(
            model_name='applicationbaselinecheck',
            name='rule_template',
            field=models.CharField(blank=True, default='', max_length=32, verbose_name='规则模板'),
            preserve_default=False,
        ),
        migrations.RunPython(bind_existing_rule_templates, migrations.RunPython.noop),
        migrations.AlterField(
            model_name='applicationbaselinecheck',
            name='rule_template',
            field=models.CharField(
                choices=[
                    ('xml_numeric_min', 'XML 数值属性下限校验'),
                    ('xml_sensitive_list', 'XML 敏感属性与列表包含校验'),
                    ('forbidden_xml_element', '禁止 XML 元素存在'),
                    ('attribute_contains_text', 'XML 属性包含文本'),
                ],
                max_length=32,
                verbose_name='规则模板',
            ),
        ),
    ]