import django.db.models.deletion
from django.db import migrations, models
from django.utils import timezone


# 原 environment 枚举值到环境实体的展示名与排序，用于把存量数据迁移成每个业务系统下的环境记录。
LEGACY_ENVIRONMENTS = {
    'production': ('生产环境', 0),
    'testing': ('测试环境', 1),
    'development': ('开发环境', 2),
    'other': ('其他环境', 9),
}


def forwards(apps, schema_editor):
    BusinessEnvironment = apps.get_model('assets', 'BusinessEnvironment')
    ApplicationService = apps.get_model('assets', 'ApplicationService')

    cache = {}
    for service in ApplicationService.objects.all().iterator():
        # 未归属业务系统的历史服务无法派生环境，保持空关联，由用户后续在页面上补齐。
        if not service.business_system_id:
            continue
        code = service.environment or 'other'
        key = (service.business_system_id, code)
        environment = cache.get(key)
        if environment is None:
            name, order = LEGACY_ENVIRONMENTS.get(code, (code, 9))
            environment, _ = BusinessEnvironment.objects.get_or_create(
                business_system_id=service.business_system_id,
                code=code,
                defaults={'name': name, 'order': order, 'create_time': timezone.now()},
            )
            cache[key] = environment
        service.environment_ref_id = environment.id
        service.save(update_fields=['environment_ref'])


def backwards(apps, schema_editor):
    ApplicationService = apps.get_model('assets', 'ApplicationService')
    for service in ApplicationService.objects.select_related('environment_ref').iterator():
        environment = service.environment_ref
        if environment is None:
            continue
        service.business_system_id = environment.business_system_id
        service.environment = environment.code
        service.save(update_fields=['business_system', 'environment'])


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0075_applicationservice_primary_deployment'),
    ]

    operations = [
        migrations.CreateModel(
            name='BusinessEnvironment',
            fields=[
                ('id', models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name='ID')),
                ('create_time', models.DateTimeField(default=timezone.now, verbose_name='创建时间')),
                ('update_time', models.DateTimeField(auto_now=True, verbose_name='修改时间')),
                ('remark', models.TextField(blank=True, default='', null=True, verbose_name='备注')),
                ('name', models.CharField(max_length=64, verbose_name='环境名称')),
                ('code', models.CharField(max_length=32, verbose_name='环境编码')),
                ('order', models.PositiveIntegerField(default=0, verbose_name='展示顺序')),
                ('owner', models.CharField(blank=True, default='', max_length=128, verbose_name='负责人')),
                ('enabled', models.BooleanField(default=True, verbose_name='是否启用')),
                ('business_system', models.ForeignKey(on_delete=django.db.models.deletion.CASCADE, related_name='environments', to='assets.businesssystem', verbose_name='所属业务系统')),
            ],
            options={
                'db_table': 'assets_business_environment',
                'ordering': ['business_system_id', 'order', 'name', 'id'],
            },
        ),
        migrations.AddConstraint(
            model_name='businessenvironment',
            constraint=models.UniqueConstraint(fields=('business_system', 'code'), name='unique_business_environment_code'),
        ),
        migrations.AddConstraint(
            model_name='businessenvironment',
            constraint=models.UniqueConstraint(fields=('business_system', 'name'), name='unique_business_environment_name'),
        ),
        migrations.RemoveConstraint(
            model_name='applicationservice',
            name='unique_business_environment_service',
        ),
        migrations.AddField(
            model_name='applicationservice',
            name='environment_ref',
            field=models.ForeignKey(blank=True, null=True, on_delete=django.db.models.deletion.PROTECT, related_name='services', to='assets.businessenvironment', verbose_name='所属环境'),
        ),
        migrations.RunPython(forwards, backwards),
        migrations.RemoveField(model_name='applicationservice', name='environment'),
        migrations.RemoveField(model_name='applicationservice', name='business_system'),
        migrations.RenameField(model_name='applicationservice', old_name='environment_ref', new_name='environment'),
        migrations.AlterModelOptions(
            name='applicationservice',
            options={'ordering': ['environment_id', 'name']},
        ),
        migrations.AddConstraint(
            model_name='applicationservice',
            constraint=models.UniqueConstraint(fields=('environment', 'name'), name='unique_business_environment_service'),
        ),
    ]
