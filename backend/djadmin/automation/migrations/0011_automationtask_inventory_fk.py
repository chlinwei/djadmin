import django.db.models.deletion
from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('automation', '0010_drop_inventory_code_column'),
    ]

    operations = [
        migrations.AddField(
            model_name='automationtask',
            name='inventory',
            field=models.ForeignKey(blank=True, null=True, on_delete=django.db.models.deletion.SET_NULL, related_name='tasks', to='automation.automationinventory'),
        ),
    ]
