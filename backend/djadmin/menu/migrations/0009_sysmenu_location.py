# Generated by Django 5.1.4 on 2025-06-04 07:04

from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('menu', '0008_alter_sysmenu_path'),
    ]

    operations = [
        migrations.AddField(
            model_name='sysmenu',
            name='location',
            field=models.SmallIntegerField(choices=[(1, 'leftMenu'), (2, 'usercenter')], default=1, verbose_name='权限位置'),
        ),
    ]
