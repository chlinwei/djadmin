from datetime import date

from django.db import migrations


def rename_log_parser_menu(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysMenu.objects.filter(path='/monitor/log-parsers').update(
        name='日志处理规则',
        update_time=date.today(),
        remark='Unified Fluent Bit preprocessing and OpenSearch ingest rule menu',
    )


def restore_log_parser_menu_name(apps, schema_editor):
    SysMenu = apps.get_model('menu', 'SysMenu')
    SysMenu.objects.filter(path='/monitor/log-parsers').update(
        name='日志解析规则',
        update_time=date.today(),
        remark='OpenSearch ingest pipeline management menu',
    )


class Migration(migrations.Migration):

    dependencies = [
        ('menu', '0048_add_log_parser_menu'),
    ]

    operations = [
        migrations.RunPython(rename_log_parser_menu, restore_log_parser_menu_name),
    ]