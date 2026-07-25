from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('monitor', '0021_add_structured_monitor_ports'),
    ]

    operations = [
        migrations.CreateModel(
            name='AlertRule',
            fields=[
                ('id', models.AutoField(auto_created=True, primary_key=True, serialize=False, verbose_name='ID')),
                ('create_time', models.DateField(auto_now_add=True, null=True)),
                ('update_time', models.DateField(auto_now=True, null=True)),
                ('group_name', models.CharField(default='host-baseline', max_length=64)),
                ('name', models.CharField(max_length=128, unique=True)),
                ('expr', models.TextField()),
                ('duration_for', models.CharField(default='2m', max_length=64)),
                ('keep_firing_for', models.CharField(blank=True, default='', max_length=64)),
                ('severity', models.CharField(choices=[('critical', 'Critical'), ('warning', 'Warning'), ('info', 'Info')], default='warning', max_length=16)),
                ('enabled', models.BooleanField(default=True)),
                ('order_num', models.PositiveIntegerField(default=100)),
                ('extra_labels', models.JSONField(blank=True, default=dict)),
                ('summary_template', models.CharField(blank=True, default='', max_length=255)),
                ('description_template', models.TextField(blank=True, default='')),
            ],
            options={
                'db_table': 'monitor_alert_rule',
                'ordering': ['order_num', '-id'],
            },
        ),
    ]
