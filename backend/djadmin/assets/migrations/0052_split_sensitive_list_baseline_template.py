from copy import deepcopy

from django.db import migrations, models


def split_sensitive_list_checks(apps, schema_editor):
    ApplicationBaselineCheck = apps.get_model('assets', 'ApplicationBaselineCheck')

    for check in ApplicationBaselineCheck.objects.filter(rule_template='xml_sensitive_list').iterator():
        rule = check.rule if isinstance(check.rule, dict) else {}
        attributes = rule.get('attributes') if isinstance(rule.get('attributes'), dict) else {}
        sensitive_attributes = {
            name: deepcopy(condition)
            for name, condition in attributes.items()
            if isinstance(condition, dict) and condition.get('sensitive')
        }
        list_attributes = {
            name: deepcopy(condition)
            for name, condition in attributes.items()
            if isinstance(condition, dict) and condition.get('operator') == 'csv_contains_all'
        }

        if sensitive_attributes and list_attributes:
            password_rule = deepcopy(rule)
            password_rule['attributes'] = sensitive_attributes
            roles_rule = deepcopy(rule)
            roles_rule['attributes'] = list_attributes

            if check.name == 'Tomcat Manager 登录用户':
                password_name = 'Tomcat Manager 登录密码'
                roles_name = 'Tomcat Manager 角色'
            else:
                password_name = f'{check.name} - 敏感属性'[:128]
                roles_name = f'{check.name} - 列表包含'[:128]

            check.name = password_name
            check.rule_template = 'xml_sensitive_attribute'
            check.rule = password_rule
            check.save(update_fields=['name', 'rule_template', 'rule'])

            ApplicationBaselineCheck.objects.create(
                application_id=check.application_id,
                name=roles_name,
                file_path=check.file_path,
                file_type=check.file_type,
                rule_template='xml_list_contains',
                rule=roles_rule,
                enabled=check.enabled,
                order=check.order + 1,
                remark=check.remark,
            )
        elif sensitive_attributes:
            check.rule_template = 'xml_sensitive_attribute'
            check.save(update_fields=['rule_template'])
        else:
            check.rule_template = 'xml_list_contains'
            check.save(update_fields=['rule_template'])


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0051_application_baseline_check_rule_template'),
    ]

    operations = [
        migrations.RunPython(split_sensitive_list_checks, migrations.RunPython.noop),
        migrations.AlterField(
            model_name='applicationbaselinecheck',
            name='rule_template',
            field=models.CharField(
                choices=[
                    ('xml_numeric_min', 'XML 数值属性下限校验'),
                    ('xml_sensitive_attribute', 'XML 敏感属性校验'),
                    ('xml_list_contains', 'XML 列表包含校验'),
                    ('forbidden_xml_element', '禁止 XML 元素存在'),
                    ('attribute_contains_text', 'XML 属性包含文本'),
                ],
                max_length=32,
                verbose_name='规则模板',
            ),
        ),
    ]