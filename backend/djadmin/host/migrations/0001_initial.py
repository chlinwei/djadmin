# Generated by Django 5.1.4 on 2025-06-07 13:06

import django.db.models.deletion
import django.utils.timezone
from django.db import migrations, models


class Migration(migrations.Migration):

    initial = True

    dependencies = [
    ]

    operations = [
        migrations.CreateModel(
            name='Host_type',
            fields=[
                ('id', models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name='ID')),
                ('create_time', models.DateTimeField(default=django.utils.timezone.now, verbose_name='创建时间')),
                ('update_time', models.DateTimeField(auto_now=True, verbose_name='修改时间')),
                ('remark', models.TextField(blank=True, default='', null=True, verbose_name='备注')),
                ('name', models.CharField(blank=True, default='', max_length=128, null=True, verbose_name='Host类别')),
            ],
            options={
                'abstract': False,
            },
        ),
        migrations.CreateModel(
            name='SSHUser',
            fields=[
                ('id', models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name='ID')),
                ('create_time', models.DateTimeField(default=django.utils.timezone.now, verbose_name='创建时间')),
                ('update_time', models.DateTimeField(auto_now=True, verbose_name='修改时间')),
                ('remark', models.TextField(blank=True, default='', null=True, verbose_name='备注')),
                ('name', models.CharField(blank=True, default='', max_length=200, null=True, verbose_name='SSH 用户')),
                ('password', models.CharField(blank=True, default='', max_length=128, null=True, verbose_name='SSH 密码')),
            ],
            options={
                'ordering': ['-id'],
                'unique_together': {('name',)},
            },
        ),
        migrations.CreateModel(
            name='Host',
            fields=[
                ('id', models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name='ID')),
                ('create_time', models.DateTimeField(default=django.utils.timezone.now, verbose_name='创建时间')),
                ('update_time', models.DateTimeField(auto_now=True, verbose_name='修改时间')),
                ('remark', models.TextField(blank=True, default='', null=True, verbose_name='备注')),
                ('hostname', models.CharField(blank=True, default='', max_length=200, null=True, verbose_name='主机名')),
                ('ssh_ip', models.CharField(blank=True, default='', max_length=128, null=True, verbose_name='SSH IP地址')),
                ('ssh_port', models.PositiveIntegerField(blank=True, default=22, null=True, verbose_name='SSH 端口')),
                ('cpu', models.CharField(blank=True, default='', max_length=64, null=True, verbose_name='CPU')),
                ('memory', models.CharField(blank=True, default='', max_length=64, null=True, verbose_name='内存')),
                ('disk', models.CharField(blank=True, default='', max_length=64, null=True, verbose_name='磁盘大小')),
                ('system_product', models.CharField(blank=True, default='', max_length=128, null=True, verbose_name='服务器类型')),
                ('status', models.CharField(choices=[('0', '下线'), ('1', '在线')], default='1', max_length=2, verbose_name='运行状态')),
                ('host_type', models.ForeignKey(blank=True, default='', null=True, on_delete=django.db.models.deletion.SET_NULL, to='host.host_type', verbose_name='主机类别')),
                ('ssh_user', models.ForeignKey(blank=True, default='', null=True, on_delete=django.db.models.deletion.SET_NULL, to='host.sshuser', verbose_name='SSH用户')),
            ],
            options={
                'ordering': ['-id'],
            },
        ),
    ]
