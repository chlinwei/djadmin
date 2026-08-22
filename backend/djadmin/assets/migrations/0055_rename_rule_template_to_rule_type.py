from django.db import migrations, models


RULE_TYPE_MAP = {
    'xml_attribute': 'xml_attribute_text',
    'xml_numeric': 'xml_attribute_number',
}


def rename_rule_type_values(apps, schema_editor):
    ApplicationBaselineCheck = apps.get_model('assets', 'ApplicationBaselineCheck')
    for old_value, new_value in RULE_TYPE_MAP.items():
        ApplicationBaselineCheck.objects.filter(rule_template=old_value).update(rule_template=new_value)


def restore_rule_template_values(apps, schema_editor):
    ApplicationBaselineCheck = apps.get_model('assets', 'ApplicationBaselineCheck')
    for old_value, new_value in RULE_TYPE_MAP.items():
        ApplicationBaselineCheck.objects.filter(rule_template=new_value).update(rule_template=old_value)


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0054_consolidate_xml_rule_templates'),
    ]

    operations = [
        migrations.RunPython(rename_rule_type_values, restore_rule_template_values),
        migrations.RenameField(
            model_name='applicationbaselinecheck',
            old_name='rule_template',
            new_name='rule_type',
        ),
        migrations.AlterField(
            model_name='applicationbaselinecheck',
            name='rule_type',
            field=models.CharField(
                choices=[
                    ('xml_attribute_text', 'XML 属性文本比较'),
                    ('xml_attribute_number', 'XML 属性数值比较'),
                    ('xml_element_absent', 'XML 元素不存在'),
                ],
                max_length=32,
                verbose_name='规则类型',
            ),
        ),
    ]