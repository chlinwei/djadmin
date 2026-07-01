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
import sys
import time
import unittest
from datetime import datetime

os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'djadmin.test_settings')

import django
django.setup()

from django.test.utils import get_runner
from django.conf import settings


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
        self.cases.append({
            'module': type(test).__module__.split('.')[0],
            'class': test.__class__.__name__,
            'name': test._testMethodName,
            'doc': (test.shortDescription() or '').strip(),
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
    TestRunner = get_runner(settings)
    runner = TestRunner(verbosity=1, keepdb=True)

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
    parser.add_argument('--apps', nargs='+', default=['user', 'role', 'assets'],
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
