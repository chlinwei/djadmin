from copy import deepcopy

from django.db import migrations, models


def remove_sensitive_rule_field(apps, schema_editor):
    ApplicationBaselineCheck = apps.get_model('assets', 'ApplicationBaselineCheck')

    for check in ApplicationBaselineCheck.objects.all().iterator():
        rule = deepcopy(check.rule) if isinstance(check.rule, dict) else {}
        attributes = rule.get('attributes')
        changed = False
        if isinstance(attributes, dict):
            for condition in attributes.values():
                if isinstance(condition, dict) and 'sensitive' in condition:
                    condition.pop('sensitive')
                    changed = True

        update_fields = []
        if changed:
            check.rule = rule
            update_fields.append('rule')
        if check.rule_template == 'xml_sensitive_attribute':
            check.rule_template = 'xml_attribute_eq'
            update_fields.append('rule_template')
        if update_fields:
            check.save(update_fields=update_fields)


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0052_split_sensitive_list_baseline_template'),
    ]

    operations = [
        migrations.RunPython(remove_sensitive_rule_field, migrations.RunPython.noop),
        migrations.AlterField(
            model_name='applicationbaselinecheck',
            name='rule_template',
            field=models.CharField(
                choices=[
                    ('xml_numeric_min', 'XML 数值属性下限校验'),
                    ('xml_attribute_eq', 'XML 属性等值校验'),
                    ('xml_list_contains', 'XML 列表包含校验'),
                    ('forbidden_xml_element', '禁止 XML 元素存在'),
                    ('attribute_contains_text', 'XML 属性包含文本'),
                ],
                max_length=32,
                verbose_name='规则模板',
            ),
        ),
    ]