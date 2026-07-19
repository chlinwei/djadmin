"""
管理命令：初始化所有系统参数默认值（幂等，已存在的不覆盖）。
用法：python manage.py init_sys_configs
"""
from django.core.management.base import BaseCommand
from sys_config.models import SysConfig


DEFAULT_CONFIGS = [
    {
        'key': 'sys.assets.collect.interval_seconds',
        'value': '40',
        'default_value': '40',
        'value_type': 'int',
        'name': '主机信息采集间隔（秒）',
        'description': 'Agent 主机信息周期上报间隔（秒）',
        'is_readonly': False,
    },
    {
        'key': 'sys.assets.hostgroup.max_tree_depth',
        'value': '5',
        'default_value': '5',
        'value_type': 'int',
        'name': '主机分组最大层级',
        'description': '主机分组树形结构的最大嵌套层数',
        'is_readonly': False,
    },
    {
        'key': 'sys.assets.host.manage.refresh_interval_seconds',
        'value': '5',
        'default_value': '5',
        'value_type': 'int',
        'name': '主机管理页刷新间隔（秒）',
        'description': '主机管理列表自动刷新间隔（秒）',
        'is_readonly': False,
    },
    {
        'key': 'sys.assets.host.detail.collect_dispatch_interval_seconds',
        'value': '8',
        'default_value': '8',
        'value_type': 'int',
        'name': '主机详情主动采集下发间隔（秒）',
        'description': '主机详情页主动下发 dj-agent 采集任务的间隔（秒）',
        'is_readonly': False,
    },
    {
        'key': 'sys.menu.max_tree_depth',
        'value': '5',
        'default_value': '5',
        'value_type': 'int',
        'name': '菜单最大层级',
        'description': '系统菜单树形结构的最大嵌套层数',
        'is_readonly': False,
    },
]


class Command(BaseCommand):
    help = '初始化系统参数默认值（幂等，已存在的不覆盖）'

    def handle(self, *args, **options):
        created_count = 0
        for item in DEFAULT_CONFIGS:
            key = item['key']
            _, created = SysConfig.objects.get_or_create(
                key=key,
                defaults={k: v for k, v in item.items() if k != 'key'},
            )
            if created:
                self.stdout.write(self.style.SUCCESS(f'  已创建: {key}'))
                created_count += 1
            else:
                self.stdout.write(f'  已存在（跳过）: {key}')

        self.stdout.write(self.style.SUCCESS(f'\n完成，新增 {created_count} 条参数。'))
