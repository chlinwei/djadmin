from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0059_add_structured_config_document_types'),
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
                    ('ini', 'INI'),
                    ('toml', 'TOML'),
                    ('properties', 'Properties'),
                    ('text', '普通文本'),
                    ('shell', 'Shell 命令'),
                ],
                default='xml',
                max_length=16,
                verbose_name='文档类型',
            ),
        ),
        migrations.AlterField(
            model_name='applicationbaselinecheck',
            name='file_path',
            field=models.CharField(blank=True, default='', max_length=512, verbose_name='文件路径'),
        ),
        migrations.AlterField(
            model_name='applicationbaselinecheck',
            name='schema_type',
            field=models.CharField(
                choices=[
                    ('schematron', 'Schematron / XPath'),
                    ('json_schema', 'JSON Schema'),
                    ('regexp', 'Regexp'),
                    ('shell', 'Shell'),
                ],
                default='schematron',
                max_length=32,
                verbose_name='Schema 类型',
            ),
        ),
    ]