# Generated by Django 5.1.4 on 2024-12-21 04:55

import django.db.models.deletion
from django.db import migrations, models


class Migration(migrations.Migration):

    initial = True

    dependencies = [
        ('menu', '0001_initial'),
        ('role', '0001_initial'),
    ]

    operations = [
        migrations.AddField(
            model_name='sysrolemenu',
            name='role',
            field=models.ForeignKey(on_delete=django.db.models.deletion.CASCADE, to='role.sysrole'),
        ),
    ]
