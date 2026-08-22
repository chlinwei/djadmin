from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0057_remove_xsd_schema_type'),
    ]

    operations = [
        migrations.AlterField(
            model_name='applicationbaselinecheck',
            name='document_type',
            field=models.CharField(
                choices=[
                    ('xml', 'XML'),
                    ('json', 'JSON'),
                    ('yaml', 'YAML'),
                    ('text', '普通文本'),
                ],
                default='xml',
                max_length=16,
                verbose_name='文档类型',
            ),
        ),
        migrations.AlterField(
            model_name='applicationbaselinecheck',
            name='schema_type',
            field=models.CharField(
                choices=[
                    ('schematron', 'Schematron / XPath'),
                    ('json_schema', 'JSON Schema'),
                    ('regexp', 'Regexp'),
                ],
                default='schematron',
                max_length=32,
                verbose_name='Schema 类型',
            ),
        ),
    ]