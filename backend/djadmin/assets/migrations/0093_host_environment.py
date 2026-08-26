import django.db.models.deletion
from django.db import migrations, models


class Migration(migrations.Migration):
    dependencies = [
        ('assets', '0092_applicationdeploymenttemplate_macro_definitions_and_more'),
    ]

    operations = [
        migrations.AddField(
            model_name='host',
            name='environment',
            field=models.ForeignKey(
                blank=True,
                null=True,
                on_delete=django.db.models.deletion.PROTECT,
                related_name='hosts',
                to='assets.businessenvironment',
                verbose_name='所属环境',
            ),
        ),
    ]
