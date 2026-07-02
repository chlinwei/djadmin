from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('audit', '0001_initial'),
    ]

    operations = [
        migrations.CreateModel(
            name='OperationAuditLog',
            fields=[
                ('id', models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name='ID')),
                ('username', models.CharField(blank=True, default='', max_length=150)),
                ('user_id', models.IntegerField(blank=True, null=True)),
                ('method', models.CharField(blank=True, default='', max_length=16)),
                ('path', models.CharField(blank=True, default='', max_length=255)),
                ('route_name', models.CharField(blank=True, default='', max_length=255)),
                ('client_ip', models.CharField(blank=True, default='', max_length=64)),
                ('user_agent', models.CharField(blank=True, default='', max_length=255)),
                ('status_code', models.IntegerField(default=200)),
                ('duration_ms', models.IntegerField(blank=True, null=True)),
                ('message', models.CharField(blank=True, default='', max_length=255)),
                ('created_at', models.DateTimeField(auto_now_add=True)),
            ],
            options={
                'db_table': 'audit_operation_log',
                'ordering': ['-created_at'],
            },
        ),
    ]