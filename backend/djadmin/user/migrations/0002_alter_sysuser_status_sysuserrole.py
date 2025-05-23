# Generated by Django 5.1.4 on 2025-05-24 08:37

import django.db.models.deletion
from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('role', '0002_delete_sysuserrole'),
        ('user', '0001_initial'),
    ]

    operations = [
        migrations.AlterField(
            model_name='sysuser',
            name='status',
            field=models.SmallIntegerField(choices=[(1, '正常'), (0, '禁用')], default=1),
        ),
        migrations.CreateModel(
            name='SysUserRole',
            fields=[
                ('id', models.AutoField(primary_key=True, serialize=False)),
                ('role', models.ForeignKey(on_delete=django.db.models.deletion.CASCADE, to='role.sysrole')),
                ('user', models.ForeignKey(on_delete=django.db.models.deletion.CASCADE, to='user.sysuser')),
            ],
            options={
                'db_table': 'sys_user_role',
                'unique_together': {('user', 'role')},
            },
        ),
    ]
