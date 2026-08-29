from django.db import migrations, models
import django.db.models.deletion


def normalize_environment_relations(apps, schema_editor):
    BusinessEnvironment = apps.get_model('assets', 'BusinessEnvironment')
    ApplicationService = apps.get_model('assets', 'ApplicationService')

    for service in ApplicationService.objects.select_related('environment').all().iterator():
        if service.environment_id is None or service.environment is None:
            raise RuntimeError(
                f'服务 {service.id} 没有关联可用于迁移的环境，请先补齐环境后再执行迁移'
            )
        if service.environment.business_system_id is None:
            raise RuntimeError(
                f'环境 {service.environment_id} 没有关联业务系统，无法迁移服务 {service.id}'
            )
        service.business_system_id = service.environment.business_system_id
        service.save(update_fields=['business_system'])

    canonical_by_code = {}
    for environment in BusinessEnvironment.objects.order_by('id').all().iterator():
        canonical = canonical_by_code.get(environment.code)
        if canonical is None:
            canonical_by_code[environment.code] = environment
            continue
        if canonical.name != environment.name:
            raise RuntimeError(
                f'环境编码 {environment.code} 对应多个名称，请先人工统一环境名称'
            )
        ApplicationService.objects.filter(environment_id=environment.id).update(
            environment_id=canonical.id
        )
        environment.delete()


def ensure_business_system_column(apps, schema_editor):
    """支持 DDL 已落库但迁移记录未写入的数据库继续执行。"""
    service_model = apps.get_model('assets', 'ApplicationService')
    table_name = service_model._meta.db_table
    column_names = {
        column.name for column in schema_editor.connection.introspection.get_table_description(
            schema_editor.connection.cursor(), table_name,
        )
    }
    if 'business_system_id' in column_names:
        return

    business_system_model = apps.get_model('assets', 'BusinessSystem')
    field = models.ForeignKey(
        business_system_model,
        on_delete=django.db.models.deletion.PROTECT,
        related_name='services',
        null=True,
        blank=True,
        verbose_name='所属业务系统',
    )
    field.set_attributes_from_name('business_system')
    field.model = service_model
    schema_editor.add_field(service_model, field)


def reconcile_partial_schema(apps, schema_editor):
    """完成 MySQL 非事务 DDL 可能留下的任意中间状态。"""
    connection = schema_editor.connection
    cursor = connection.cursor()
    service_table = 'assets_application_service'
    environment_table = 'assets_business_environment'

    def columns(table):
        cursor.execute(f'SHOW COLUMNS FROM `{table}`')
        return {row[0] for row in cursor.fetchall()}

    def indexes(table):
        cursor.execute(f'SHOW INDEX FROM `{table}`')
        return {row[2] for row in cursor.fetchall()}

    def has_index_for_column(table, column_name):
        cursor.execute(f'SHOW INDEX FROM `{table}`')
        return any(row[1] == 1 and row[4] == column_name and row[3] == 1 for row in cursor.fetchall())

    service_columns = columns(service_table)
    environment_columns = columns(environment_table)
    if 'business_system_id' not in service_columns:
        cursor.execute(
            f'ALTER TABLE `{service_table}` ADD COLUMN `business_system_id` BIGINT NULL'
        )

    if 'business_system_id' in environment_columns:
        cursor.execute(
            f'UPDATE `{service_table}` s JOIN `{environment_table}` e '
            'ON s.environment_id = e.id SET s.business_system_id = e.business_system_id '
            'WHERE s.business_system_id IS NULL'
        )
    cursor.execute(
        f'SELECT COUNT(*) FROM `{service_table}` WHERE business_system_id IS NULL'
    )
    if cursor.fetchone()[0]:
        raise RuntimeError('存在无法确定所属业务系统的逻辑服务，迁移已停止')

    # 必须先摘掉 business_system_id 外键：旧唯一索引以该列结尾，MySQL 会以「索引被外键依赖」拒绝 DROP INDEX。
    if 'business_system_id' in environment_columns:
        cursor.execute(
            f'SHOW CREATE TABLE `{environment_table}`'
        )
        create_sql = cursor.fetchone()[1]
        for line in create_sql.splitlines():
            if 'FOREIGN KEY (`business_system_id`)' in line:
                constraint_name = line.split('CONSTRAINT `', 1)[1].split('`', 1)[0]
                cursor.execute(
                    f'ALTER TABLE `{environment_table}` DROP FOREIGN KEY `{constraint_name}`'
                )
        cursor.execute(f'ALTER TABLE `{environment_table}` DROP COLUMN `business_system_id`')

    environment_indexes = indexes(environment_table)
    for index_name in ('unique_business_environment_code', 'unique_business_environment_name'):
        if index_name in environment_indexes:
            cursor.execute(f'ALTER TABLE `{environment_table}` DROP INDEX `{index_name}`')
    if 'unique_business_environment_code' not in indexes(environment_table):
        cursor.execute(
            f'ALTER TABLE `{environment_table}` ADD UNIQUE KEY '
            '`unique_business_environment_code` (`code`)'
        )
    if 'unique_business_environment_name' not in indexes(environment_table):
        cursor.execute(
            f'ALTER TABLE `{environment_table}` ADD UNIQUE KEY '
            '`unique_business_environment_name` (`name`)'
        )

    service_indexes = indexes(service_table)
    if not has_index_for_column(service_table, 'environment_id'):
        cursor.execute(
            f'ALTER TABLE `{service_table}` ADD INDEX '
            '`assets_application_service_environment_id_idx` (`environment_id`)'
        )
    if 'unique_business_environment_service' in service_indexes:
        cursor.execute(
            f'ALTER TABLE `{service_table}` DROP INDEX `unique_business_environment_service`'
        )
    cursor.execute(f'ALTER TABLE `{service_table}` MODIFY `business_system_id` BIGINT NOT NULL')
    if 'unique_business_environment_service' not in indexes(service_table):
        cursor.execute(
            f'ALTER TABLE `{service_table}` ADD UNIQUE KEY '
            '`unique_business_environment_service` '
            '(`business_system_id`, `environment_id`, `name`)'
        )


def noop_reverse(apps, schema_editor):
    # 合并后的全局环境无法无损还原为原业务系统范围内的多条记录。
    pass


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0088_deployment_config_single_source'),
    ]

    operations = [
        migrations.SeparateDatabaseAndState(
            database_operations=[migrations.RunPython(reconcile_partial_schema, migrations.RunPython.noop)],
            state_operations=[
                migrations.AddField(
                    model_name='applicationservice',
                    name='business_system',
                    field=models.ForeignKey(
                        blank=True,
                        null=True,
                        on_delete=django.db.models.deletion.PROTECT,
                        related_name='services',
                        to='assets.businesssystem',
                        verbose_name='所属业务系统',
                    ),
                ),
                migrations.RemoveConstraint(model_name='businessenvironment', name='unique_business_environment_code'),
                migrations.RemoveConstraint(model_name='businessenvironment', name='unique_business_environment_name'),
                migrations.RemoveField(model_name='businessenvironment', name='business_system'),
                migrations.AlterModelOptions(name='businessenvironment', options={'ordering': ['order', 'name', 'id']}),
                migrations.AddConstraint(model_name='businessenvironment', constraint=models.UniqueConstraint(fields=('code',), name='unique_business_environment_code')),
                migrations.AddConstraint(model_name='businessenvironment', constraint=models.UniqueConstraint(fields=('name',), name='unique_business_environment_name')),
                migrations.AlterField(
                    model_name='applicationservice', name='business_system',
                    field=models.ForeignKey(on_delete=django.db.models.deletion.PROTECT, related_name='services', to='assets.businesssystem', verbose_name='所属业务系统'),
                ),
                migrations.AlterModelOptions(name='applicationservice', options={'ordering': ['business_system_id', 'environment_id', 'name']}),
                migrations.RemoveConstraint(model_name='applicationservice', name='unique_business_environment_service'),
                migrations.AddConstraint(model_name='applicationservice', constraint=models.UniqueConstraint(fields=('business_system', 'environment', 'name'), name='unique_business_environment_service')),
            ],
        ),
    ]
