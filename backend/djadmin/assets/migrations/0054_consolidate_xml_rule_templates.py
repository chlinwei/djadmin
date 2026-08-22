from copy import deepcopy

from django.db import migrations, models


TEMPLATE_MAP = {
    'xml_numeric_min': 'xml_numeric',
    'xml_attribute_eq': 'xml_attribute',
    'xml_list_contains': 'xml_attribute',
    'attribute_contains_text': 'xml_attribute',
    'forbidden_xml_element': 'xml_element_absent',
}

OPERATOR_MAP = {
    'contains_ci': 'contains',
    'gte_number': 'gte',
}


def consolidate_xml_templates(apps, schema_editor):
    ApplicationBaselineCheck = apps.get_model('assets', 'ApplicationBaselineCheck')

    for check in ApplicationBaselineCheck.objects.all().iterator():
        rule = deepcopy(check.rule) if isinstance(check.rule, dict) else {}
        for section_name in ('match', 'attributes'):
            section = rule.get(section_name)
            if not isinstance(section, dict):
                continue
            for condition in section.values():
                if not isinstance(condition, dict):
                    continue
                operator = condition.get('operator')
                if operator in OPERATOR_MAP:
                    condition['operator'] = OPERATOR_MAP[operator]

        check.rule_template = TEMPLATE_MAP.get(check.rule_template, check.rule_template)
        check.rule = rule
        check.save(update_fields=['rule_template', 'rule'])


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0053_remove_sensitive_xml_rule'),
    ]

    operations = [
        migrations.RunPython(consolidate_xml_templates, migrations.RunPython.noop),
        migrations.AlterField(
            model_name='applicationbaselinecheck',
            name='rule_template',
            field=models.CharField(
                choices=[
                    ('xml_attribute', 'XML 属性值'),
                    ('xml_numeric', 'XML 数值'),
                    ('xml_element_absent', 'XML 元素不存在'),
                ],
                max_length=32,
                verbose_name='规则模板',
            ),
        ),
    ]