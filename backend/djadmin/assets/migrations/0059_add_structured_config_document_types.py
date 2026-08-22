from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0058_add_regexp_baseline_type'),
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
                ],
                default='xml',
                max_length=16,
                verbose_name='文档类型',
            ),
        ),
    ]