from xml.sax.saxutils import escape, quoteattr

from django.db import migrations, models


UPPERCASE = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ'
LOWERCASE = 'abcdefghijklmnopqrstuvwxyz'


def xpath_literal(value):
    text = str(value)
    if "'" not in text:
        return f"'{text}'"
    if '"' not in text:
        return f'"{text}"'
    parts = text.split("'")
    return 'concat(' + ', "\'", '.join(f"'{part}'" for part in parts) + ')'


def attribute_condition(attribute, condition):
    attribute_ref = f'@{attribute}'
    operator = condition.get('operator')
    value = condition.get('value')
    if operator == 'eq':
        if isinstance(value, (int, float)) and not isinstance(value, bool):
            return f'number({attribute_ref}) = {value}'
        return f'{attribute_ref} = {xpath_literal(value)}'
    if operator == 'contains':
        expected = xpath_literal(str(value).lower())
        return f"contains(translate({attribute_ref}, '{UPPERCASE}', '{LOWERCASE}'), {expected})"
    if operator == 'csv_contains_all':
        normalized = f"concat(',', translate({attribute_ref}, ' ', ''), ',')"
        return ' and '.join(
            f"contains({normalized}, {xpath_literal(',' + str(item) + ',')})"
            for item in value
        )
    if operator in {'gte', 'gt', 'lt', 'lte'}:
        xpath_operator = {'gte': '>=', 'gt': '>', 'lt': '<', 'lte': '<='}[operator]
        comparisons = [f'({attribute_ref} and number({attribute_ref}) {xpath_operator} {value})']
        comparisons.extend(f'{attribute_ref} = {xpath_literal(item)}' for item in condition.get('unlimited_values', []))
        default = condition.get('default')
        if default is not None and {
            'gte': default >= value,
            'gt': default > value,
            'lt': default < value,
            'lte': default <= value,
        }[operator]:
            comparisons.append(f'not({attribute_ref})')
        return '(' + ' or '.join(comparisons) + ')'
    raise ValueError(f'无法迁移操作符 {operator}，请先清理对应基线检查项')


def compile_schematron(check):
    rule = check.rule
    element = str(rule.get('element') or '')
    selector = f"//*[local-name() = {xpath_literal(element)}]"
    match_conditions = [attribute_condition(name, condition) for name, condition in rule.get('match', {}).items()]
    if match_conditions:
        selector += '[' + ' and '.join(match_conditions) + ']'

    if rule.get('assertion') == 'absent':
        test = f'not({selector})'
    else:
        assertions = [attribute_condition(name, condition) for name, condition in rule.get('attributes', {}).items()]
        assertion = ' and '.join(assertions) or 'true()'
        test = f'boolean({selector}) and not({selector}[not({assertion})])'

    title = escape(check.name)
    message = escape(f'{check.name} 不符合配置基线')
    return (
        '<schema xmlns="http://purl.oclc.org/dsdl/schematron" queryBinding="xslt">\n'
        f'  <title>{title}</title>\n'
        '  <pattern id="application-baseline">\n'
        '    <rule context="/">\n'
        f'      <assert test={quoteattr(test)}>{message}</assert>\n'
        '    </rule>\n'
        '  </pattern>\n'
        '</schema>'
    )


def migrate_rules_to_schemas(apps, schema_editor):
    ApplicationBaselineCheck = apps.get_model('assets', 'ApplicationBaselineCheck')
    for check in ApplicationBaselineCheck.objects.all().iterator():
        check.document_type = 'xml'
        check.schema_type = 'schematron'
        check.schema_version = 'iso'
        check.schema_content = compile_schematron(check)
        check.save(update_fields=['document_type', 'schema_type', 'schema_version', 'schema_content'])


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0055_rename_rule_template_to_rule_type'),
    ]

    operations = [
        migrations.AddField(
            model_name='applicationbaselinecheck',
            name='document_type',
            field=models.CharField(choices=[('xml', 'XML'), ('json', 'JSON'), ('yaml', 'YAML')], default='xml', max_length=16, verbose_name='文档类型'),
        ),
        migrations.AddField(
            model_name='applicationbaselinecheck',
            name='schema_content',
            field=models.TextField(default='', verbose_name='Schema 内容'),
            preserve_default=False,
        ),
        migrations.AddField(
            model_name='applicationbaselinecheck',
            name='schema_type',
            field=models.CharField(choices=[('xsd', 'XSD'), ('schematron', 'Schematron'), ('json_schema', 'JSON Schema')], default='schematron', max_length=32, verbose_name='Schema 类型'),
        ),
        migrations.AddField(
            model_name='applicationbaselinecheck',
            name='schema_version',
            field=models.CharField(default='iso', max_length=32, verbose_name='Schema 版本'),
        ),
        migrations.RunPython(migrate_rules_to_schemas, migrations.RunPython.noop),
        migrations.RemoveField(model_name='applicationbaselinecheck', name='file_type'),
        migrations.RemoveField(model_name='applicationbaselinecheck', name='rule'),
        migrations.RemoveField(model_name='applicationbaselinecheck', name='rule_type'),
    ]