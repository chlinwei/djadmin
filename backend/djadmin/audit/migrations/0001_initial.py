from django.db import migrations, models


class Migration(migrations.Migration):

    initial = True

    dependencies = []

    operations = [
        migrations.CreateModel(
            name='LoginAuditLog',
            fields=[
                ('id', models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name='ID')),
                ('username', models.CharField(blank=True, default='', max_length=150)),
                ('user_id', models.IntegerField(blank=True, null=True)),
                ('status', models.CharField(choices=[('success', '登录成功'), ('failed', '登录失败')], default='success', max_length=16)),
                ('client_ip', models.CharField(blank=True, default='', max_length=64)),
                ('user_agent', models.CharField(blank=True, default='', max_length=255)),
                ('message', models.CharField(blank=True, default='', max_length=255)),
                ('login_time', models.DateTimeField(auto_now_add=True)),
            ],
            options={
                'db_table': 'audit_login_log',
                'ordering': ['-login_time'],
            },
        ),
    ]
