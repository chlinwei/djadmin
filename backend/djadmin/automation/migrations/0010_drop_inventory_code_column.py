from django.db import migrations


def drop_code_column_if_exists(apps, schema_editor):
    connection = schema_editor.connection
    table_name = 'automation_inventory'
    column_name = 'code'
    with connection.cursor() as cursor:
        cursor.execute(
            """
            SELECT COUNT(*)
            FROM information_schema.COLUMNS
            WHERE TABLE_SCHEMA = DATABASE()
              AND TABLE_NAME = %s
              AND COLUMN_NAME = %s
            """,
            [table_name, column_name],
        )
        exists = cursor.fetchone()[0] > 0

        if exists:
            cursor.execute(f'ALTER TABLE {table_name} DROP COLUMN {column_name}')


def add_code_column_if_missing(apps, schema_editor):
    connection = schema_editor.connection
    table_name = 'automation_inventory'
    column_name = 'code'
    with connection.cursor() as cursor:
        cursor.execute(
            """
            SELECT COUNT(*)
            FROM information_schema.COLUMNS
            WHERE TABLE_SCHEMA = DATABASE()
              AND TABLE_NAME = %s
              AND COLUMN_NAME = %s
            """,
            [table_name, column_name],
        )
        exists = cursor.fetchone()[0] > 0

        if not exists:
            cursor.execute(
                f'ALTER TABLE {table_name} ADD COLUMN {column_name} varchar(128) NULL UNIQUE'
            )


class Migration(migrations.Migration):

    dependencies = [
        ('automation', '0009_automationinventory'),
    ]

    operations = [
        migrations.RunPython(drop_code_column_if_exists, add_code_column_if_missing),
    ]
