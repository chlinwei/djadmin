#!/usr/bin/env python
"""
测试报告生成器
运行 Django 测试套件并生成 Markdown 格式的测试报告。

报告包含每个用例的：所属模块、用例名、说明、耗时、结果。

用法:
    python generate_test_report.py
    python generate_test_report.py --apps user role assets
    python generate_test_report.py --output ../../TEST_REPORT.md
"""
import argparse
import io
import os
import re
import sys
import time
import unittest
from datetime import datetime

os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'djadmin.test_settings')

import django
django.setup()

from django.db import connections
from django.test.utils import get_runner
from django.conf import settings

import MySQLdb


def fallback_description_from_test_name(test_name):
    """基于测试方法名生成可读说明，避免报告中出现大量 '-'。"""
    if not test_name:
        return '-'

    name = str(test_name)
    if name.startswith('test_'):
        name = name[5:]
    name = name.strip('_')
    if not name:
        return '-'

    return name.replace('_', ' ')


def contains_chinese(text):
    return bool(re.search(r'[\u4e00-\u9fff]', text or ''))


def _translate_token_to_cn(token):
    token_map = {
        'admin': '管理员',
        'all': '全部',
        'and': '并且',
        'are': '被',
        'as': '为',
        'attachment': '附件',
        'audit': '审计',
        'batch': '批量',
        'by': '按',
        'cancel': '取消',
        'celery': 'Celery',
        'cleanup': '清理',
        'completed': '完成',
        'config': '配置',
        'connect': '连接',
        'contains': '包含',
        'content': '内容',
        'create': '创建',
        'created': '创建',
        'default': '默认',
        'dispatches': '派发',
        'dispatch': '派发',
        'download': '下载',
        'entries': '条目',
        'error': '错误',
        'execution': '执行',
        'exists': '存在',
        'extension': '扩展名',
        'falls': '回退',
        'fallback': '回退',
        'file': '文件',
        'filter': '过滤',
        'filtered': '已过滤',
        'filters': '过滤',
        'finished': '已完成',
        'for': '用于',
        'found': '不存在',
        'from': '来自',
        'get': '获取',
        'has': '有',
        'ids': 'ID 列表',
        'in': '在',
        'invalid': '非法',
        'job': '任务',
        'keyword': '关键字',
        'list': '列表',
        'login': '登录',
        'log': '日志',
        'logs': '日志',
        'menu': '菜单',
        'middleware': '中间件',
        'missing': '缺失',
        'more': '更多',
        'now': '立即',
        'offline': '离线',
        'on': '在',
        'online': '在线',
        'operation': '操作',
        'output': '输出',
        'paginated': '分页',
        'patch': '更新',
        'permission': '权限',
        'permissions': '权限',
        'playbook': 'Playbook',
        'rejects': '拒绝',
        'removes': '移除',
        'respects': '遵循',
        'retention': '保留',
        'retrieve': '查询',
        'returns': '返回',
        'retries': '重试',
        'role': '角色',
        'run': '执行',
        'running': '运行中',
        'sanitized': '脱敏',
        'scope': '范围',
        'search': '搜索',
        'selected': '选中',
        'sends': '发送',
        'session': '会话',
        'sessions': '会话',
        'snapshot': '快照',
        'status': '状态',
        'submits': '提交',
        'success': '成功',
        'successful': '成功',
        'supports': '支持',
        'summary': '摘要',
        'task': '任务',
        'template': '模板',
        'three': '三个',
        'to': '到',
        'token': '令牌',
        'tree': '树',
        'two': '两个',
        'update': '更新',
        'upload': '上传',
        'uses': '使用',
        'validate': '校验',
        'view': '查看',
        'webssh': 'WebSSH',
        'when': '当',
        'window': '窗口',
        'with': '带',
        'without': '不带',
        'worker': 'Worker',
        'yaml': 'YAML',
        'zip': '压缩包',
    }
    return token_map.get(token, token)


def english_text_to_cn_style(text):
    normalized = (text or '').strip().lower()
    if not normalized:
        return ''

    normalized = normalized.replace('-', ' ').replace('_', ' ')

    phrase_map = {
        'not found': '未找到',
        'falls back to': '回退到',
        'fall back to': '回退到',
        'run now': '立即执行',
        'job list': '任务列表',
        'when worker offline': '当 Worker 离线',
        'when worker online': '当 Worker 在线',
    }
    for src, dst in phrase_map.items():
        normalized = normalized.replace(src, dst)

    tokens = [t for t in re.split(r'\s+', normalized) if t]
    if not tokens:
        return ''

    translated = []
    for tok in tokens:
        if contains_chinese(tok):
            translated.append(tok)
            continue
        translated.append(_translate_token_to_cn(tok))

    sentence = ' '.join(translated).strip()
    if not sentence:
        return ''
    return f'验证：{sentence}'


def normalize_description_to_chinese(doc, test_name):
    """统一说明为中文风格：优先保留中文，英文说明自动转中文模板。"""
    raw = (doc or '').strip()
    if not raw:
        raw = fallback_description_from_test_name(test_name)

    if contains_chinese(raw):
        return raw

    converted = english_text_to_cn_style(raw)
    if converted:
        return converted
    return f'验证：{raw}'


class TimingTestResult(unittest.TextTestResult):
    """记录每个用例的耗时与结果。"""

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.cases = []
        self._start = None

    def startTest(self, test):
        self._start = time.perf_counter()
        super().startTest(test)

    def _record(self, test, status):
        elapsed = (time.perf_counter() - self._start) if self._start else 0.0
        doc = normalize_description_to_chinese(test.shortDescription(), test._testMethodName)

        self.cases.append({
            'module': type(test).__module__.split('.')[0],
            'class': test.__class__.__name__,
            'name': test._testMethodName,
            'doc': doc,
            'duration': elapsed,
            'status': status,
        })

    def addSuccess(self, test):
        super().addSuccess(test)
        self._record(test, 'ok')

    def addFailure(self, test, err):
        super().addFailure(test, err)
        self._record(test, 'FAIL')

    def addError(self, test, err):
        super().addError(test, err)
        self._record(test, 'ERROR')

    def addSkip(self, test, reason):
        super().addSkip(test, reason)
        self._record(test, 'skipped')


def run_tests(app_labels):
    """运行测试并收集结果。返回 (result, total, duration)。"""
    _reset_test_database()

    TestRunner = get_runner(settings)
    runner = TestRunner(verbosity=1, keepdb=False, interactive=False)

    suite = runner.build_suite(app_labels)
    total = suite.countTestCases()

    stream = io.StringIO()
    runner_obj = unittest.TextTestRunner(
        stream=stream, verbosity=1, resultclass=TimingTestResult,
    )

    old_config = runner.setup_databases()
    start = time.perf_counter()
    try:
        result = runner_obj.run(suite)
    finally:
        runner.teardown_databases(old_config)
    duration = time.perf_counter() - start
    return result, total, duration


def _reset_test_database():
    """删除并重建固定测试库，避免 Django 询问 database exists。"""
    db_config = settings.DATABASES.get('default', {})
    test_name = db_config.get('TEST', {}).get('NAME') or db_config.get('NAME')
    if not test_name:
        return

    host = db_config.get('HOST') or 'localhost'
    port = int(db_config.get('PORT') or 3306)
    user = db_config.get('USER') or 'root'
    password = db_config.get('PASSWORD') or ''

    connections.close_all()
    connection = MySQLdb.connect(
        host=host,
        port=port,
        user=user,
        passwd=password,
        charset='utf8mb4',
    )
    try:
        connection.autocommit(True)
        with connection.cursor() as cursor:
            cursor.execute(f'DROP DATABASE IF EXISTS `{test_name}`')
    finally:
        connection.close()


def status_icon(status):
    return {
        'ok': '✅ 通过',
        'FAIL': '❌ 失败',
        'ERROR': '🔥 错误',
        'skipped': '⏭️ 跳过',
    }.get(status, status)


def generate_markdown(result, total, duration):
    now = datetime.now().strftime('%Y-%m-%d %H:%M:%S')
    cases = result.cases
    failures = len(result.failures)
    errors = len(result.errors)
    skipped = len(result.skipped)
    passed = total - failures - errors - skipped
    pass_rate = (passed / total * 100) if total else 0
    overall = '✅ 全部通过' if result.wasSuccessful() else '❌ 存在失败'

    lines = []
    lines.append('# 测试报告')
    lines.append('')
    lines.append(f'- **生成时间**: {now}')
    lines.append(f'- **整体结果**: {overall}')
    lines.append(f'- **总耗时**: {duration:.3f} 秒')
    lines.append('')
    lines.append('## 汇总')
    lines.append('')
    lines.append('| 指标 | 数量 |')
    lines.append('| --- | --- |')
    lines.append(f'| 总用例数 | {total} |')
    lines.append(f'| ✅ 通过 | {passed} |')
    lines.append(f'| ❌ 失败 | {failures} |')
    lines.append(f'| 🔥 错误 | {errors} |')
    lines.append(f'| ⏭️ 跳过 | {skipped} |')
    lines.append(f'| 通过率 | {pass_rate:.1f}% |')
    lines.append('')

    # 按模块汇总
    module_stats = {}
    for case in cases:
        s = module_stats.setdefault(case['module'], {'count': 0, 'duration': 0.0})
        s['count'] += 1
        s['duration'] += case['duration']

    lines.append('## 模块汇总')
    lines.append('')
    lines.append('| 模块 | 用例数 | 耗时(秒) |')
    lines.append('| --- | --- | --- |')
    for mod in sorted(module_stats):
        s = module_stats[mod]
        lines.append(f"| {mod} | {s['count']} | {s['duration']:.3f} |")
    lines.append('')

    # 用例明细：模块 / 测试类 / 用例 / 说明 / 耗时 / 结果
    lines.append('## 用例明细')
    lines.append('')
    lines.append('| 模块 | 测试类 | 用例 | 说明 | 耗时(秒) | 结果 |')
    lines.append('| --- | --- | --- | --- | --- | --- |')
    for case in sorted(cases, key=lambda c: (c['module'], c['class'], c['name'])):
        doc = case['doc'] or '-'
        lines.append(
            f"| {case['module']} | {case['class']} | {case['name']} | {doc} "
            f"| {case['duration']:.3f} | {status_icon(case['status'])} |"
        )
    lines.append('')

    # 失败和错误详情
    if result.failures or result.errors:
        lines.append('## 失败 / 错误详情')
        lines.append('')
        for test, trace in result.failures:
            lines.append(f'### ❌ {test}')
            lines.append('')
            lines.append('```')
            lines.append(trace.strip())
            lines.append('```')
            lines.append('')
        for test, trace in result.errors:
            lines.append(f'### 🔥 {test}')
            lines.append('')
            lines.append('```')
            lines.append(trace.strip())
            lines.append('```')
            lines.append('')

    return '\n'.join(lines)


def main():
    parser = argparse.ArgumentParser(description='生成 Markdown 测试报告')
    parser.add_argument('--apps', nargs='+', default=['user', 'role', 'menu', 'assets', 'audit', 'automation', 'scheduler'],
                        help='要测试的 app 列表')
    parser.add_argument('--output', default=os.path.join('..', '..', 'TEST_REPORT.md'),
                        help='报告输出路径')
    args = parser.parse_args()

    print(f'运行测试: {", ".join(args.apps)} ...')
    result, total, duration = run_tests(args.apps)
    markdown = generate_markdown(result, total, duration)

    output_path = os.path.abspath(args.output)
    with open(output_path, 'w', encoding='utf-8') as f:
        f.write(markdown)

    passed = total - len(result.failures) - len(result.errors) - len(result.skipped)
    print(f'\n完成：{passed}/{total} 通过，总耗时 {duration:.3f}s')
    print(f'报告已生成: {output_path}')
    sys.exit(0 if result.wasSuccessful() else 1)


if __name__ == '__main__':
    main()
