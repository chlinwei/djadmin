from django.db import migrations, models
import django.db.models.deletion
import django.utils.timezone


def seed_default_group_and_backfill(apps, schema_editor):
    AlertRule = apps.get_model('monitor', 'AlertRule')
    AlertRuleGroup = apps.get_model('monitor', 'AlertRuleGroup')

    default_group, _ = AlertRuleGroup.objects.get_or_create(
        name='host-baseline',
        defaults={
            'interval': '30s',
            'enabled': True,
            'order_num': 100,
            'remark': 'default group',
        },
    )

    for rule in AlertRule.objects.all():
        group_name = str(getattr(rule, 'group_name', '') or '').strip() or 'host-baseline'
        group, _ = AlertRuleGroup.objects.get_or_create(
            name=group_name,
            defaults={
                'interval': '30s',
                'enabled': True,
                'order_num': 100,
                'remark': 'auto created from legacy group_name',
            },
        )
        rule.group_id = group.id
        rule.save(update_fields=['group'])

    if not AlertRule.objects.filter(group_id__isnull=False).exists():
        for rule in AlertRule.objects.all():
            rule.group_id = default_group.id
            rule.save(update_fields=['group'])


class Migration(migrations.Migration):

    dependencies = [
        ('monitor', '0022_alertrule'),
    ]

    operations = [
        migrations.CreateModel(
            name='AlertRuleGroup',
            fields=[
                ('id', models.AutoField(auto_created=True, primary_key=True, serialize=False, verbose_name='ID')),
                ('create_time', models.DateTimeField(default=django.utils.timezone.now, verbose_name='创建时间')),
                ('update_time', models.DateTimeField(auto_now=True, verbose_name='修改时间')),
                ('remark', models.TextField(blank=True, default='', null=True, verbose_name='备注')),
                ('name', models.CharField(max_length=64, unique=True)),
                ('interval', models.CharField(default='30s', max_length=64)),
                ('enabled', models.BooleanField(default=True)),
                ('order_num', models.PositiveIntegerField(default=100)),
            ],
            options={
                'db_table': 'monitor_alert_rule_group',
                'ordering': ['order_num', '-id'],
            },
        ),
        migrations.CreateModel(
            name='AlertRuleDeployHistory',
            fields=[
                ('id', models.AutoField(auto_created=True, primary_key=True, serialize=False, verbose_name='ID')),
                ('create_time', models.DateTimeField(default=django.utils.timezone.now, verbose_name='创建时间')),
                ('update_time', models.DateTimeField(auto_now=True, verbose_name='修改时间')),
                ('remark', models.TextField(blank=True, default='', null=True, verbose_name='备注')),
                ('status', models.CharField(choices=[('success', 'Success'), ('failed', 'Failed')], default='success', max_length=16)),
                ('deployed_file_path', models.CharField(blank=True, default='', max_length=512)),
                ('backup_file_path', models.CharField(blank=True, default='', max_length=512)),
                ('reload_url', models.CharField(blank=True, default='', max_length=512)),
                ('message', models.TextField(blank=True, default='')),
                ('yaml_snapshot', models.TextField(blank=True, default='')),
                ('requested_user_id_snapshot', models.IntegerField(blank=True, default=None, null=True)),
                ('requested_username_snapshot', models.CharField(blank=True, default='', max_length=100)),
            ],
            options={
                'db_table': 'monitor_alert_rule_deploy_history',
                'ordering': ['-id'],
            },
        ),
        migrations.AddField(
            model_name='alertrule',
            name='group',
            field=models.ForeignKey(blank=True, null=True, on_delete=django.db.models.deletion.SET_NULL, related_name='rules', to='monitor.alertrulegroup'),
        ),
        migrations.AlterField(
            model_name='alertrule',
            name='create_time',
            field=models.DateTimeField(default=django.utils.timezone.now, verbose_name='创建时间'),
        ),
        migrations.AlterField(
            model_name='alertrule',
            name='update_time',
            field=models.DateTimeField(auto_now=True, verbose_name='修改时间'),
        ),
        migrations.RunPython(seed_default_group_and_backfill, migrations.RunPython.noop),
    ]
