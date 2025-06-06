# Generated by Django 5.1.4 on 2025-06-01 10:51

from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('menu', '0003_alter_sysmenu_table'),
        ('role', '0004_alter_sysrole_create_time_alter_sysrole_update_time'),
    ]

    operations = [
        migrations.AlterField(
            model_name='sysmenu',
            name='create_time',
            field=models.DateField(blank=True, null=True, verbose_name='创建时间'),
        ),
        migrations.AlterField(
            model_name='sysmenu',
            name='remark',
            field=models.CharField(blank=True, max_length=500, null=True, verbose_name='备注'),
        ),
        migrations.AlterField(
            model_name='sysmenu',
            name='update_time',
            field=models.DateField(blank=True, null=True, verbose_name='更新时间'),
        ),
        migrations.AlterUniqueTogether(
            name='sysrolemenu',
            unique_together={('menu', 'role')},
        ),
    ]
