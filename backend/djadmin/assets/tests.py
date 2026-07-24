from django.test import TestCase
from django.core.files.uploadedfile import SimpleUploadedFile
from unittest.mock import AsyncMock, MagicMock, patch
from contextlib import contextmanager
from typing import Any, cast
from rest_framework.test import APIClient
from rest_framework_jwt.settings import api_settings
from .models import Credential, Application, HostGroup, Host, HostCredential, HostDisk, HostHardware, WebSSHSessionLog
from .consumers import HostWebSSHConsumer
from .webssh_runtime import WebSSHRuntimeRegistry
from monitor.models import MonitorTarget, SoftwarePackage
from automation.models import PlaybookTemplate
from automation.models import TemplateCategory
from user.models import SysUser


def _make_playbook_template(name):
    """安装/卸载 Playbook 模板测试夹具：直接创建一个“软件包安装/卸载专用”分类的 PlaybookTemplate，
    供 SoftwarePackage.install_playbook_template/uninstall_playbook_template 绑定使用
    （安装/卸载不再经由 AutomationTask 中转，见 monitor.models.SoftwarePackage 注释）。"""
    return PlaybookTemplate.objects.create(
        name=f'{name}-template',
        content='- hosts: all\n  tasks:\n    - debug:\n        msg: noop\n',
        category=TemplateCategory.SOFTWARE_PACKAGE,
    )



def _get_token(user: SysUser) -> str:
    jwt_payload_handler = api_settings.JWT_PAYLOAD_HANDLER  # type: ignore[operator]
    jwt_encode_handler = api_settings.JWT_ENCODE_HANDLER  # type: ignore[operator]
    payload = jwt_payload_handler(user)  # type: ignore[operator]
    return jwt_encode_handler(payload)  # type: ignore[operator]


class BaseTestCase(TestCase):
    def setUp(self):
        self.client = APIClient()
        self.user = SysUser.objects.create(
            username='admin', password='admin123', status=1
        )
        token = _get_token(self.user)
        self.client.credentials(HTTP_AUTHORIZATION=token)

    def assertResponseOK(self, res):
        """校验响应符合统一返回格式 {code, msg, data} 且 code==200。

        所有业务用例都通过本方法断言，因此格式契约由业务测试顺带覆盖，
        无需单独的格式测试文件。注意：msg 的具体值不强制为 'success'，
        因为部分业务（如采集失败）成功返回 code=200 但 msg 描述错误信息。
        """
        body = res.json()
        self.assertIn('code', body, msg=f"响应缺少 'code' 字段: {body}")
        self.assertIn('msg', body, msg=f"响应缺少 'msg' 字段: {body}")
        self.assertIn('data', body, msg=f"响应缺少 'data' 字段: {body}")
        self.assertEqual(body['code'], 200, msg=f"Expected code=200, got: {body}")
        return body

    def assertSuccess(self, res):
        """比 assertResponseOK 更严格：额外要求 msg == 'success'，
        用于纯增删改查成功场景。"""
        body = self.assertResponseOK(res)
        self.assertEqual(body['msg'], 'success',
                         msg=f"Expected msg='success', got: {body['msg']}")
        return body


# ─────────────────────────────────────────────
# 凭证管理
# ─────────────────────────────────────────────
class CredentialTest(BaseTestCase):

    def test_list_credentials_paginated(self):
        """凭证列表返回分页格式"""
        Credential.objects.create(name='cred1', username='root', port=22, auth_type=1)
        res = self.client.get('/assets/credentials/?page=1&page_size=10')
        body = self.assertResponseOK(res)
        self.assertIn('results', body['data'])
        self.assertIn('count', body['data'])
        self.assertIn('pageSize', body['data'])
        self.assertIn('totalPages', body['data'])

    def test_create_credential(self):
        """新增凭证"""
        res = self.client.post('/assets/credentials/', {
            'name': 'new_cred',
            'username': 'root',
            'password': 'secret',
            'port': 22,
            'auth_type': 1,
        }, format='json')
        self.assertResponseOK(res)
        self.assertTrue(Credential.objects.filter(name='new_cred').exists())

    def test_get_credential_detail(self):
        """获取凭证详情格式正确"""
        cred = Credential.objects.create(name='detail_cred', username='root', port=22, auth_type=1)
        res = self.client.get(f'/assets/credentials/{cred.id}/')  # type: ignore[attr-defined]
        body = self.assertResponseOK(res)
        self.assertEqual(body['data']['name'], 'detail_cred')

    def test_update_credential(self):
        """编辑凭证"""
        cred = Credential.objects.create(name='old_name', username='root', port=22, auth_type=1)
        res = self.client.patch(f'/assets/credentials/{cred.id}/', {  # type: ignore[attr-defined]
            'name': 'new_name',
        }, format='json')
        self.assertResponseOK(res)
        cred.refresh_from_db()
        self.assertEqual(cred.name, 'new_name')

    def test_batch_delete_credentials(self):
        """批量删除凭证"""
        cred = Credential.objects.create(name='to_delete', username='root', port=22, auth_type=1)
        res = self.client.delete('/assets/credentials/batch-delete/', {
            'ids': [cred.id]  # type: ignore[attr-defined]
        }, format='json')
        self.assertResponseOK(res)
        self.assertFalse(Credential.objects.filter(id=cred.id).exists())  # type: ignore[attr-defined]

    def test_search_credentials(self):
        """搜索凭证"""
        Credential.objects.create(name='match_me', username='root', port=22, auth_type=1)
        Credential.objects.create(name='other', username='root', port=22, auth_type=1)
        res = self.client.get('/assets/credentials/?search=match_me')
        body = self.assertResponseOK(res)
        self.assertEqual(body['data']['count'], 1)
        self.assertEqual(body['data']['results'][0]['name'], 'match_me')

    def test_list_without_token(self):
        """无 token 返回 301"""
        client = APIClient()
        res = client.get('/assets/credentials/')
        self.assertEqual(res.json()['code'], 301)  # type: ignore[attr-defined]

    def test_update_credential_connection_config_should_force_close_related_webssh_sessions(self):
        """修改凭证连接配置后，应断开使用该默认凭证的主机 WebSSH 会话。"""
        credential = Credential.objects.create(
            name='cred-close-session',
            username='root',
            password='secret',
            port=22,
            auth_type=Credential.AuthType.PASSWORD,
        )
        host = Host.objects.create(instance_name='cred_host', ip='192.168.1.120')
        HostCredential.objects.create(host=host, credential=credential, is_default=True)
        session = WebSSHSessionLog.objects.create(
            host=host,
            user_id=self.user.id,
            username=self.user.username,
            status=WebSSHSessionLog.Status.CONNECTED,
        )
        fake_consumer = MagicMock()
        fake_consumer._send_event = AsyncMock(return_value=None)
        fake_consumer.close = AsyncMock(return_value=None)
        WebSSHRuntimeRegistry.mark_active(session.id, consumer=fake_consumer, host_id=host.id)  # type: ignore[arg-type]
        try:
            res = self.client.patch(
                f'/assets/credentials/{credential.id}/',  # type: ignore[attr-defined]
                {'password': 'new-secret'},
                format='json',
            )
            self.assertResponseOK(res)
            fake_consumer.close.assert_awaited_once()
            self.assertEqual(WebSSHRuntimeRegistry.get_active_count_for_host(host.id), 0)
        finally:
            WebSSHRuntimeRegistry.mark_inactive(session.id)  # type: ignore[arg-type]

    def test_update_credential_non_connection_fields_should_not_force_close_sessions(self):
        """仅修改凭证展示字段（如名称）时，不应误断开在线 WebSSH 会话。"""
        credential = Credential.objects.create(
            name='cred-keep-session',
            username='root',
            password='secret',
            port=22,
            auth_type=Credential.AuthType.PASSWORD,
        )
        host = Host.objects.create(instance_name='cred_host_keep', ip='192.168.1.121')
        HostCredential.objects.create(host=host, credential=credential, is_default=True)
        session = WebSSHSessionLog.objects.create(
            host=host,
            user_id=self.user.id,
            username=self.user.username,
            status=WebSSHSessionLog.Status.CONNECTED,
        )
        fake_consumer = MagicMock()
        fake_consumer._send_event = AsyncMock(return_value=None)
        fake_consumer.close = AsyncMock(return_value=None)
        WebSSHRuntimeRegistry.mark_active(session.id, consumer=fake_consumer, host_id=host.id)  # type: ignore[arg-type]
        try:
            res = self.client.patch(
                f'/assets/credentials/{credential.id}/',  # type: ignore[attr-defined]
                {'name': 'cred-keep-session-renamed'},
                format='json',
            )
            self.assertResponseOK(res)
            fake_consumer.close.assert_not_awaited()
            self.assertEqual(WebSSHRuntimeRegistry.get_active_count_for_host(host.id), 1)
        finally:
            WebSSHRuntimeRegistry.mark_inactive(session.id)  # type: ignore[arg-type]


# ─────────────────────────────────────────────
# 应用管理
# ─────────────────────────────────────────────
class ApplicationTest(BaseTestCase):

    def test_list_applications(self):
        """应用列表返回分页格式"""
        Application.objects.create(name='app1', version='1.0')
        res = self.client.get('/assets/applications/?page=1&page_size=10')
        body = self.assertResponseOK(res)
        self.assertIn('results', body['data'])

    def test_create_application(self):
        """新增应用"""
        res = self.client.post('/assets/applications/', {
            'name': 'new_app', 'version': '2.0'
        }, format='json')
        self.assertResponseOK(res)
        self.assertTrue(Application.objects.filter(name='new_app').exists())

    def test_batch_delete_applications(self):
        """批量删除应用"""
        app = Application.objects.create(name='del_app', version='1.0')
        res = self.client.delete('/assets/applications/batch-delete/', {
            'ids': [app.id]  # type: ignore[attr-defined]
        }, format='json')
        self.assertResponseOK(res)
        self.assertFalse(Application.objects.filter(id=app.id).exists())  # type: ignore[attr-defined]


# ─────────────────────────────────────────────
# 主机分组
# ─────────────────────────────────────────────
class HostGroupTest(BaseTestCase):

    def test_list_host_groups(self):
        """主机分组列表"""
        HostGroup.objects.create(name='group1')
        res = self.client.get('/assets/host-groups/?page=1&page_size=10')
        body = self.assertResponseOK(res)
        self.assertIn('results', body['data'])

    def test_create_host_group(self):
        """新增主机分组"""
        res = self.client.post('/assets/host-groups/', {
            'name': 'new_group'
        }, format='json')
        self.assertResponseOK(res)
        self.assertTrue(HostGroup.objects.filter(name='new_group').exists())

    def test_update_host_group(self):
        """编辑主机分组"""
        group = HostGroup.objects.create(name='old_group')
        res = self.client.patch(f'/assets/host-groups/{group.id}/', {  # type: ignore[attr-defined]
            'name': 'renamed_group'
        }, format='json')
        self.assertResponseOK(res)
        group.refresh_from_db()
        self.assertEqual(group.name, 'renamed_group')

    def test_get_host_group_detail(self):
        """获取主机分组详情"""
        group = HostGroup.objects.create(name='detail_group')
        res = self.client.get(f'/assets/host-groups/{group.id}/')  # type: ignore[attr-defined]
        body = self.assertResponseOK(res)
        self.assertEqual(body['data']['name'], 'detail_group')

    def test_get_host_group_tree(self):
        """获取分组树形结构"""
        parent = HostGroup.objects.create(name='parent_group')
        HostGroup.objects.create(name='child_group', parent=parent)
        res = self.client.get('/assets/host-groups/tree/')
        body = self.assertResponseOK(res)
        self.assertIsInstance(body['data'], list)

    def test_create_nested_group(self):
        """创建嵌套子分组"""
        parent = HostGroup.objects.create(name='root')
        res = self.client.post('/assets/host-groups/', {
            'name': 'child', 'parent_id': parent.id  # type: ignore[attr-defined]  # serializer 用 parent_id 字段
        }, format='json')
        self.assertResponseOK(res)
        child = HostGroup.objects.get(name='child')
        self.assertIsNotNone(child.parent)
        self.assertEqual(child.parent.id, parent.id)  # type: ignore[attr-defined,union-attr]

    def test_delete_host_group(self):
        """删除主机分组"""
        group = HostGroup.objects.create(name='del_group')
        res = self.client.delete(f'/assets/host-groups/{group.id}/')  # type: ignore[attr-defined]
        self.assertResponseOK(res)
        self.assertFalse(HostGroup.objects.filter(id=group.id).exists())  # type: ignore[attr-defined]


# ─────────────────────────────────────────────
# 主机管理
# ─────────────────────────────────────────────
class HostTest(BaseTestCase):

    @contextmanager
    def _active_webssh_session(self, host):
        session = WebSSHSessionLog.objects.create(
            host=host,
            user_id=self.user.id,
            username=self.user.username,
            status=WebSSHSessionLog.Status.CONNECTED,
        )
        WebSSHRuntimeRegistry.mark_active(session.id, host_id=host.id)  # type: ignore[arg-type]
        try:
            yield
        finally:
            WebSSHRuntimeRegistry.mark_inactive(session.id)  # type: ignore[arg-type]

    def test_list_hosts(self):
        """主机列表返回分页格式"""
        Host.objects.create(instance_name='host1', ip='192.168.1.1')
        res = self.client.get('/assets/hosts/?page=1&page_size=10')
        body = self.assertResponseOK(res)
        self.assertIn('results', body['data'])
        self.assertIn('count', body['data'])

    def test_list_hosts_with_host_id_filter(self):
        """按主机 ID 过滤应精确返回单台主机"""
        target = Host.objects.create(instance_name='test3', ip='192.168.1.10')
        Host.objects.create(instance_name='test3-bak', ip='192.168.1.11')
        res = self.client.get(f'/assets/hosts/?host_id={target.id}')
        body = self.assertResponseOK(res)
        self.assertEqual(body['data']['count'], 1)
        self.assertEqual(body['data']['results'][0]['id'], target.id)

    def test_create_host(self):
        """新增主机"""
        group = HostGroup.objects.create(name='hg')
        res = self.client.post('/assets/hosts/', {
            'instance_name': 'new_host',
            'ip': '192.168.1.100',
            'group_id': group.id,  # type: ignore[attr-defined]
        }, format='json')
        self.assertResponseOK(res)
        self.assertTrue(Host.objects.filter(instance_name='new_host').exists())

    @patch('automation.local_runner.run_job_in_background', return_value=None)
    def test_create_host_with_monitors_payload_should_enqueue_exporter_install(self, _mock_run_job):
        """新增主机时通过 monitors 数组纳管 node_exporter 并开启，应下发安装用 automation job。"""
        install_template = _make_playbook_template('node_exporter-install')
        SoftwarePackage.objects.create(
            name='node_exporter', version='9.9.9', os='linux', arch='amd64', enabled=True,
            install_playbook_template=install_template,
        )
        res = self.client.post('/assets/hosts/', {
            'instance_name': 'agent-host-01',
            'ip': '192.168.1.101',
            'monitors': [{'name': 'node_exporter', 'enabled': True}],
        }, format='json')
        self.assertResponseOK(res)

        host = Host.objects.get(instance_name='agent-host-01')
        target = MonitorTarget.objects.get(host=host, exporter_type=MonitorTarget.ExporterType.NODE_EXPORTER)
        self.assertTrue(target.managed_enabled)
        self.assertEqual(target.install_status, MonitorTarget.InstallStatus.PENDING)
        self.assertTrue('已下发安装任务' in target.install_message)
        self.assertIsNone(target.last_install_job_id)

    def test_dispatch_exporter_install_job_service_run_as_user_defaults_to_dj_agent(self):
        """未显式指定 service_run_as_user/group 时，模型层默认值 dj-agent 会直接落库，
        extra_vars 应原样透传该默认值（与 dj-agent 自身运行账号保持一致）。"""
        from assets.views import dispatch_exporter_install_job

        install_template = _make_playbook_template('node_exporter-install-default')
        SoftwarePackage.objects.create(
            name='node_exporter', version='9.9.9', os='linux', arch='amd64', enabled=True,
            install_playbook_template=install_template,
        )
        host = Host.objects.create(instance_name='agent-host-default', ip='192.168.1.198')
        target = MonitorTarget.objects.create(host=host, exporter_type='node_exporter', managed_enabled=True)

        with patch('automation.local_runner.run_job_in_background', return_value=None):
            dispatch_exporter_install_job(host, target)

        target.refresh_from_db()
        self.assertEqual(target.install_status, MonitorTarget.InstallStatus.PENDING)
        self.assertEqual(target.last_install_job_id, None)

    def test_dispatch_exporter_install_job_fallback_for_legacy_blank_value(self):
        """兼容迁移前遗留的空字符串记录（绕过 ORM default，直接 update 出空值模拟历史脏数据）：
        dispatch 时应 fallback 到 dj-agent，而不是把空字符串透传给安装 Playbook。"""
        from assets.views import dispatch_exporter_install_job

        install_template = _make_playbook_template('node_exporter-install-legacy-blank')
        pkg = SoftwarePackage.objects.create(
            name='node_exporter', version='9.9.9', os='linux', arch='amd64', enabled=True,
            install_playbook_template=install_template,
        )
        # 绕开模型默认值，模拟迁移前遗留的空字符串脏数据
        SoftwarePackage.objects.filter(id=pkg.id).update(service_run_as_user='', service_run_as_group='')
        host = Host.objects.create(instance_name='agent-host-legacy-blank', ip='192.168.1.196')
        target = MonitorTarget.objects.create(host=host, exporter_type='node_exporter', managed_enabled=True)

        with patch('automation.local_runner.run_job_in_background', return_value=None):
            dispatch_exporter_install_job(host, target)

        target.refresh_from_db()
        self.assertEqual(target.install_status, MonitorTarget.InstallStatus.PENDING)
        self.assertEqual(target.last_install_job_id, None)

    def test_dispatch_exporter_install_job_uses_explicit_service_run_as_user(self):
        """显式配置了 service_run_as_user/service_run_as_group 时，extra_vars 应原样透传，不触发 fallback。"""
        from assets.views import dispatch_exporter_install_job

        install_template = _make_playbook_template('node_exporter-install-explicit')
        SoftwarePackage.objects.create(
            name='node_exporter', version='9.9.9', os='linux', arch='amd64', enabled=True,
            install_playbook_template=install_template,
            service_run_as_user='monitor_agent', service_run_as_group='monitor_group',
        )
        host = Host.objects.create(instance_name='agent-host-explicit', ip='192.168.1.197')
        target = MonitorTarget.objects.create(host=host, exporter_type='node_exporter', managed_enabled=True)

        with patch('automation.local_runner.run_job_in_background', return_value=None):
            dispatch_exporter_install_job(host, target)

        target.refresh_from_db()
        self.assertEqual(target.install_status, MonitorTarget.InstallStatus.PENDING)
        self.assertEqual(target.last_install_job_id, None)

    def test_dispatch_exporter_install_job_without_local_package_should_reject(self):
        """dispatch_exporter_install_job 在本地软件仓库没有该 exporter 的启用包时应直接拒绝下发，
        不应创建任何 automation job。使用 node_exporter 以外的 exporter 名称，
        避开迁移预置的默认 node_exporter 软件包，确保命中“本地无包”这一分支。"""
        from assets.views import dispatch_exporter_install_job

        host = Host.objects.create(instance_name='agent-host-no-pkg', ip='192.168.1.199')
        target = MonitorTarget.objects.create(
            host=host,
            exporter_type='custom_exporter_without_pkg',
            managed_enabled=True,
        )
        dispatch_exporter_install_job(host, target)

        target.refresh_from_db()
        self.assertEqual(target.install_status, MonitorTarget.InstallStatus.FAILED)
        self.assertIn('本地软件仓库缺少', target.install_message)
        self.assertIsNone(target.last_install_job_id)

    def test_create_host_without_monitors_payload_should_not_create_monitor_target(self):
        """未提交 monitors 数组时不应自动创建任何监控目标（不再有默认 node_exporter 隐式行为）。"""
        res = self.client.post('/assets/hosts/', {
            'instance_name': 'agent-host-00',
            'ip': '192.168.1.100',
        }, format='json')
        self.assertResponseOK(res)

        host = Host.objects.get(instance_name='agent-host-00')
        self.assertFalse(MonitorTarget.objects.filter(host=host).exists())

    @patch('automation.local_runner.run_job_in_background', return_value=None)
    def test_create_host_monitor_disabled_should_not_enqueue_install(self, _mock_run_job):
        """monitors 数组中 enabled=False 时，不下发安装任务。"""
        install_template = _make_playbook_template('node_exporter-install-2')
        SoftwarePackage.objects.create(
            name='node_exporter', version='9.9.9', os='linux', arch='amd64', enabled=True,
            install_playbook_template=install_template,
        )
        res = self.client.post('/assets/hosts/', {
            'instance_name': 'agent-host-02',
            'ip': '192.168.1.102',
            'monitors': [{'name': 'node_exporter', 'enabled': False}],
        }, format='json')
        self.assertResponseOK(res)

        host = Host.objects.get(instance_name='agent-host-02')
        target = MonitorTarget.objects.get(host=host, exporter_type=MonitorTarget.ExporterType.NODE_EXPORTER)
        self.assertFalse(target.managed_enabled)
        self.assertFalse('已下发安装任务' in target.install_message)

    @patch('automation.local_runner.run_job_in_background', return_value=None)
    def test_disable_monitor_should_always_enqueue_uninstall_job(self, _mock_run_job):
        """监控从开启切到关闭时，应始终自动下发卸载任务（不再需要额外勾选一次性指令）。"""
        install_template = _make_playbook_template('node_exporter-install-3')
        uninstall_template = _make_playbook_template('node_exporter-uninstall-3')
        SoftwarePackage.objects.create(
            name='node_exporter', version='9.9.9', os='linux', arch='amd64', enabled=True,
            install_playbook_template=install_template, uninstall_playbook_template=uninstall_template,
        )
        create_res = self.client.post('/assets/hosts/', {
            'instance_name': 'agent-host-03',
            'ip': '192.168.1.103',
            'monitors': [{'name': 'node_exporter', 'enabled': True}],
        }, format='json')
        self.assertResponseOK(create_res)
        host_id = create_res.json()['data']['id']

        # 新链路下安装/卸载都不再创建 AutomationExecutionJob，手动把上一次状态置为 success，
        # 避免 disable 时被 "卸载任务已存在（pending）" 分支短路。
        host = Host.objects.get(id=host_id)
        pre_target = MonitorTarget.objects.get(host=host, exporter_type=MonitorTarget.ExporterType.NODE_EXPORTER)
        pre_target.install_status = MonitorTarget.InstallStatus.SUCCESS
        pre_target.save(update_fields=['install_status', 'update_time'])

        patch_res = self.client.patch(f'/assets/hosts/{host_id}/', {
            'monitors': [{'name': 'node_exporter', 'enabled': False}],
        }, format='json')
        self.assertResponseOK(patch_res)

        host = Host.objects.get(id=host_id)
        target = MonitorTarget.objects.get(host=host, exporter_type=MonitorTarget.ExporterType.NODE_EXPORTER)
        self.assertFalse(target.managed_enabled)
        self.assertEqual(target.install_status, MonitorTarget.InstallStatus.PENDING)
        self.assertTrue('已下发卸载任务' in target.install_message)
        self.assertEqual(target.last_install_job_id, None)

    def test_get_host_detail(self):
        """获取主机详情"""
        host = Host.objects.create(instance_name='detail_host', ip='192.168.1.1')
        res = self.client.get(f'/assets/hosts/{host.id}/')  # type: ignore[attr-defined]
        body = self.assertResponseOK(res)
        self.assertEqual(body['data']['instance_name'], 'detail_host')

    def test_get_host_detail_ignores_sr0_in_disks(self):
        """主机详情应隐藏 /dev/sr0 与 squashfs 磁盘，且使用率仅按有效磁盘计算。"""
        host = Host.objects.create(instance_name='detail_disk_host', ip='192.168.1.20')
        HostHardware.objects.create(
            host=host,
            cpu_cores=4,
            cpu_model='Intel',
            memory_gb=8,
            disk_total_gb=101,
            architecture='x86_64',
        )
        HostDisk.objects.create(host=host, device='/dev/sda1', mount_point='/', size_gb=100, used_gb=40, filesystem='ext4')
        HostDisk.objects.create(host=host, device='/dev/sr0', mount_point='/media/cdrom', size_gb=1, used_gb=1, filesystem='iso9660')
        HostDisk.objects.create(host=host, device='/dev/loop0', mount_point='/snap/core20', size_gb=2, used_gb=2, filesystem='squashfs')

        res = self.client.get(f'/assets/hosts/{host.id}/')  # type: ignore[attr-defined]
        body = self.assertResponseOK(res)
        self.assertEqual(len(body['data']['disks']), 1)
        self.assertEqual(body['data']['disks'][0]['device'], '/dev/sda1')
        self.assertEqual(body['data']['disk_used_percent'], 40.0)

    def test_update_host(self):
        """编辑主机后返回完整主机信息"""
        host = Host.objects.create(instance_name='old_host', ip='192.168.1.1')
        res = self.client.patch(f'/assets/hosts/{host.id}/', {  # type: ignore[attr-defined]
            'instance_name': 'renamed_host'
        }, format='json')
        body = self.assertResponseOK(res)
        self.assertIn('id', body['data'])
        self.assertEqual(body['data']['instance_name'], 'renamed_host')
        host.refresh_from_db()
        self.assertEqual(host.instance_name, 'renamed_host')

    def test_delete_host_should_force_close_active_webssh_sessions(self):
        """删除主机时应主动关闭该主机在线 WebSSH 会话。"""
        host = Host.objects.create(instance_name='ws_host_del', ip='192.168.1.200')
        session = WebSSHSessionLog.objects.create(
            host=host,
            user_id=self.user.id,
            username=self.user.username,
            status=WebSSHSessionLog.Status.CONNECTED,
        )
        fake_consumer = MagicMock()
        fake_consumer._send_event = AsyncMock(return_value=None)
        fake_consumer.close = AsyncMock(return_value=None)

        WebSSHRuntimeRegistry.mark_active(session.id, consumer=fake_consumer, host_id=host.id)  # type: ignore[arg-type]
        try:
            res = self.client.delete(f'/assets/hosts/{host.id}/')  # type: ignore[attr-defined]
            self.assertResponseOK(res)
            self.assertFalse(Host.objects.filter(id=host.id).exists())  # type: ignore[attr-defined]
            fake_consumer._send_event.assert_awaited_once()
            fake_consumer.close.assert_awaited_once()
            self.assertEqual(WebSSHRuntimeRegistry.get_active_count_for_host(host.id), 0)
        finally:
            WebSSHRuntimeRegistry.mark_inactive(session.id)  # type: ignore[arg-type]

    def test_batch_delete_hosts_with_mixed_ids_should_close_only_target_active_sessions(self):
        """批量删除混合合法/非法 id 时，只关闭合法且在线目标主机会话。"""
        host_a = Host.objects.create(instance_name='ws_batch_a', ip='192.168.2.10')
        host_b = Host.objects.create(instance_name='ws_batch_b', ip='192.168.2.11')
        host_c = Host.objects.create(instance_name='ws_batch_c', ip='192.168.2.12')

        session_a = WebSSHSessionLog.objects.create(
            host=host_a,
            user_id=self.user.id,
            username=self.user.username,
            status=WebSSHSessionLog.Status.CONNECTED,
        )
        session_b = WebSSHSessionLog.objects.create(
            host=host_b,
            user_id=self.user.id,
            username=self.user.username,
            status=WebSSHSessionLog.Status.CONNECTED,
        )
        session_c = WebSSHSessionLog.objects.create(
            host=host_c,
            user_id=self.user.id,
            username=self.user.username,
            status=WebSSHSessionLog.Status.CONNECTED,
        )

        fake_consumer_a = MagicMock()
        fake_consumer_a._send_event = AsyncMock(return_value=None)
        fake_consumer_a.close = AsyncMock(return_value=None)

        fake_consumer_b = MagicMock()
        fake_consumer_b._send_event = AsyncMock(return_value=None)
        fake_consumer_b.close = AsyncMock(return_value=None)

        fake_consumer_c = MagicMock()
        fake_consumer_c._send_event = AsyncMock(return_value=None)
        fake_consumer_c.close = AsyncMock(return_value=None)

        WebSSHRuntimeRegistry.mark_active(session_a.id, consumer=fake_consumer_a, host_id=host_a.id)  # type: ignore[arg-type]
        WebSSHRuntimeRegistry.mark_active(session_b.id, consumer=fake_consumer_b, host_id=host_b.id)  # type: ignore[arg-type]
        WebSSHRuntimeRegistry.mark_active(session_c.id, consumer=fake_consumer_c, host_id=host_c.id)  # type: ignore[arg-type]

        try:
            res = self.client.delete(
                '/assets/hosts/batch-delete/',
                {
                    # 仅 host_a / host_b 是有效目标；其余为非法或无效值
                    'ids': [host_a.id, 'x', '', None, '999999', host_b.id, -1, 0],
                },
                format='json',
            )
            body = self.assertResponseOK(res)
            self.assertEqual(body['data']['deleted_count'], 2)

            # 目标主机会话被关闭
            fake_consumer_a.close.assert_awaited_once()
            fake_consumer_b.close.assert_awaited_once()

            # 非目标主机会话不受影响
            fake_consumer_c.close.assert_not_awaited()

            self.assertFalse(Host.objects.filter(id=host_a.id).exists())  # type: ignore[attr-defined]
            self.assertFalse(Host.objects.filter(id=host_b.id).exists())  # type: ignore[attr-defined]
            self.assertTrue(Host.objects.filter(id=host_c.id).exists())  # type: ignore[attr-defined]

            self.assertEqual(WebSSHRuntimeRegistry.get_active_count_for_host(host_a.id), 0)
            self.assertEqual(WebSSHRuntimeRegistry.get_active_count_for_host(host_b.id), 0)
            self.assertEqual(WebSSHRuntimeRegistry.get_active_count_for_host(host_c.id), 1)
        finally:
            WebSSHRuntimeRegistry.mark_inactive(session_a.id)  # type: ignore[arg-type]
            WebSSHRuntimeRegistry.mark_inactive(session_b.id)  # type: ignore[arg-type]
            WebSSHRuntimeRegistry.mark_inactive(session_c.id)  # type: ignore[arg-type]

    def test_update_host_ip_should_force_close_active_webssh_sessions(self):
        """修改主机 IP 后，应主动断开该主机在线 WebSSH 会话。"""
        host = Host.objects.create(instance_name='ws_host_ip', ip='192.168.1.210')
        session = WebSSHSessionLog.objects.create(
            host=host,
            user_id=self.user.id,
            username=self.user.username,
            status=WebSSHSessionLog.Status.CONNECTED,
        )
        fake_consumer = MagicMock()
        fake_consumer._send_event = AsyncMock(return_value=None)
        fake_consumer.close = AsyncMock(return_value=None)
        WebSSHRuntimeRegistry.mark_active(session.id, consumer=fake_consumer, host_id=host.id)  # type: ignore[arg-type]
        try:
            res = self.client.patch(
                f'/assets/hosts/{host.id}/',  # type: ignore[attr-defined]
                {'ip': '192.168.1.211'},
                format='json',
            )
            self.assertResponseOK(res)
            fake_consumer.close.assert_awaited_once()
            self.assertEqual(WebSSHRuntimeRegistry.get_active_count_for_host(host.id), 0)
        finally:
            WebSSHRuntimeRegistry.mark_inactive(session.id)  # type: ignore[arg-type]

    def test_update_host_non_connection_fields_should_not_force_close_active_webssh_sessions(self):
        """仅修改主机展示字段（如名称）时，不应误断开在线 WebSSH 会话。"""
        host = Host.objects.create(instance_name='ws_host_no_close', ip='192.168.1.240')
        session = WebSSHSessionLog.objects.create(
            host=host,
            user_id=self.user.id,
            username=self.user.username,
            status=WebSSHSessionLog.Status.CONNECTED,
        )
        fake_consumer = MagicMock()
        fake_consumer._send_event = AsyncMock(return_value=None)
        fake_consumer.close = AsyncMock(return_value=None)
        WebSSHRuntimeRegistry.mark_active(session.id, consumer=fake_consumer, host_id=host.id)  # type: ignore[arg-type]
        try:
            res = self.client.patch(
                f'/assets/hosts/{host.id}/',  # type: ignore[attr-defined]
                {'instance_name': 'ws_host_no_close_renamed'},
                format='json',
            )
            self.assertResponseOK(res)
            fake_consumer.close.assert_not_awaited()
            self.assertEqual(WebSSHRuntimeRegistry.get_active_count_for_host(host.id), 1)
        finally:
            WebSSHRuntimeRegistry.mark_inactive(session.id)  # type: ignore[arg-type]

    def test_list_webssh_sessions(self):
        """可按主机查询 Web SSH 会话审计记录"""
        host = Host.objects.create(instance_name='ws_host', ip='192.168.1.2')
        WebSSHSessionLog.objects.create(
            host=host,
            user_id=self.user.id,
            username=self.user.username,
            status=WebSSHSessionLog.Status.CLOSED,
            command_count=3,
            input_bytes=30,
        )
        res = self.client.get(f'/assets/hosts/{host.id}/webssh-sessions/?page=1&page_size=10')  # type: ignore[attr-defined]
        body = self.assertResponseOK(res)
        self.assertIn('results', body['data'])
        self.assertEqual(body['data']['count'], 1)
        self.assertEqual(body['data']['results'][0]['host_name'], 'ws_host')

    def test_get_webssh_active_count(self):
        """可查看某台主机当前在线 WebSSH 连接数"""
        host_a = Host.objects.create(instance_name='ws_host_a', ip='192.168.1.21')
        host_b = Host.objects.create(instance_name='ws_host_b', ip='192.168.1.22')
        session_a_1 = WebSSHSessionLog.objects.create(host=host_a, user_id=self.user.id, username=self.user.username)
        session_a_2 = WebSSHSessionLog.objects.create(host=host_a, user_id=self.user.id, username=self.user.username)
        session_b = WebSSHSessionLog.objects.create(host=host_b, user_id=self.user.id, username=self.user.username)

        WebSSHRuntimeRegistry.mark_active(session_a_1.id, host_id=host_a.id)  # type: ignore[arg-type]
        WebSSHRuntimeRegistry.mark_active(session_a_2.id, host_id=host_a.id)  # type: ignore[arg-type]
        WebSSHRuntimeRegistry.mark_active(session_b.id, host_id=host_b.id)  # type: ignore[arg-type]
        try:
            res = self.client.get(f'/assets/hosts/{host_a.id}/webssh-active-count/')  # type: ignore[attr-defined]
            body = self.assertResponseOK(res)
            self.assertEqual(body['data']['host_id'], host_a.id)
            self.assertEqual(body['data']['active_count'], 2)
        finally:
            WebSSHRuntimeRegistry.mark_inactive(session_a_1.id)  # type: ignore[arg-type]
            WebSSHRuntimeRegistry.mark_inactive(session_a_2.id)  # type: ignore[arg-type]
            WebSSHRuntimeRegistry.mark_inactive(session_b.id)  # type: ignore[arg-type]

    def test_get_webssh_active_sessions(self):
        """可查看某台主机在线会话的用户名和开始时间"""
        host = Host.objects.create(instance_name='ws_host_c', ip='192.168.1.23')
        active_log = WebSSHSessionLog.objects.create(
            host=host,
            user_id=self.user.id,
            username='online_user',
            status=WebSSHSessionLog.Status.CONNECTED,
        )
        closed_log = WebSSHSessionLog.objects.create(
            host=host,
            user_id=self.user.id,
            username='offline_user',
            status=WebSSHSessionLog.Status.CLOSED,
        )
        WebSSHRuntimeRegistry.mark_active(active_log.id, host_id=host.id)  # type: ignore[arg-type]
        WebSSHRuntimeRegistry.mark_active(closed_log.id, host_id=host.id)  # type: ignore[arg-type]
        try:
            res = self.client.get(f'/assets/hosts/{host.id}/webssh-active-sessions/')  # type: ignore[attr-defined]
            body = self.assertResponseOK(res)
            self.assertEqual(body['data']['host_id'], host.id)
            self.assertEqual(body['data']['active_count'], 1)
            self.assertEqual(len(body['data']['sessions']), 1)
            self.assertEqual(body['data']['sessions'][0]['username'], 'online_user')
            self.assertEqual(body['data']['sessions'][0]['id'], active_log.id)
            self.assertIn('start_time', body['data']['sessions'][0])
        finally:
            WebSSHRuntimeRegistry.mark_inactive(active_log.id)  # type: ignore[arg-type]
            WebSSHRuntimeRegistry.mark_inactive(closed_log.id)  # type: ignore[arg-type]

    def test_list_webssh_files(self):
        """可获取主机文件列表（目录优先排序）。"""
        host = Host.objects.create(instance_name='ws_host_files', ip='192.168.1.24')

        class _Entry:
            def __init__(self, name, is_dir, size, mtime):
                self.name = name
                self.is_dir = is_dir
                self.size = size
                self.mtime = mtime

        class _ListResp:
            current_path = '/root'
            entries = [
                _Entry('b.txt', False, 20, 1710000000),
                _Entry('apps', True, 0, 1710000100),
            ]

        fake_grpc_client = MagicMock()
        fake_grpc_client.list_dir.return_value = _ListResp()

        with self._active_webssh_session(host):
            with patch('assets.views.HostManage._get_agent_grpc_client', return_value=fake_grpc_client):
                res = self.client.get(f'/assets/hosts/{host.id}/files/list/?path=/root')  # type: ignore[attr-defined]
                body = self.assertResponseOK(res)
                self.assertEqual(body['data']['current_path'], '/root')
                self.assertEqual(body['data']['entries'][0]['name'], 'apps')
                self.assertTrue(body['data']['entries'][0]['is_dir'])
                self.assertEqual(body['data']['entries'][1]['name'], 'b.txt')

    def test_rename_webssh_file(self):
        """可重命名主机文件。"""
        host = Host.objects.create(instance_name='ws_host_rename', ip='192.168.1.25')

        class _RenameResp:
            path = '/root/new.txt'
            name = 'new.txt'

        fake_grpc_client = MagicMock()
        fake_grpc_client.rename.return_value = _RenameResp()
        with self._active_webssh_session(host):
            with patch('assets.views.HostManage._get_agent_grpc_client', return_value=fake_grpc_client):
                res = self.client.post(
                    f'/assets/hosts/{host.id}/files/rename/',  # type: ignore[attr-defined]
                    {'path': '/root/old.txt', 'new_name': 'new.txt'},
                    format='json',
                )
                body = self.assertResponseOK(res)
                self.assertEqual(body['data']['name'], 'new.txt')
                fake_grpc_client.rename.assert_called_once_with('/root/old.txt', 'new.txt')

    def test_create_webssh_directory(self):
        """可创建远端目录。"""
        host = Host.objects.create(instance_name='ws_host_mkdir', ip='192.168.1.26')

        class _MkdirResp:
            path = '/root/logs'
            name = 'logs'

        fake_grpc_client = MagicMock()
        fake_grpc_client.mkdir.return_value = _MkdirResp()
        with self._active_webssh_session(host):
            with patch('assets.views.HostManage._get_agent_grpc_client', return_value=fake_grpc_client):
                res = self.client.post(
                    f'/assets/hosts/{host.id}/files/create-dir/',  # type: ignore[attr-defined]
                    {'path': '/root', 'name': 'logs'},
                    format='json',
                )
                body = self.assertResponseOK(res)
                self.assertEqual(body['data']['path'], '/root/logs')
                fake_grpc_client.mkdir.assert_called_once_with('/root', 'logs')

    def test_create_webssh_empty_file(self):
        """可创建远端空文件。"""
        host = Host.objects.create(instance_name='ws_host_touch', ip='192.168.1.27')

        class _CreateFileResp:
            path = '/root/empty.txt'
            name = 'empty.txt'

        fake_grpc_client = MagicMock()
        fake_grpc_client.create_file.return_value = _CreateFileResp()
        with self._active_webssh_session(host):
            with patch('assets.views.HostManage._get_agent_grpc_client', return_value=fake_grpc_client):
                res = self.client.post(
                    f'/assets/hosts/{host.id}/files/create-file/',  # type: ignore[attr-defined]
                    {'path': '/root', 'name': 'empty.txt'},
                    format='json',
                )
                body = self.assertResponseOK(res)
                self.assertEqual(body['data']['path'], '/root/empty.txt')
                fake_grpc_client.create_file.assert_called_once_with('/root', 'empty.txt')

# ─────────────────────────────────────────────
# WebSSH Consumer
# ─────────────────────────────────────────────
class HostWebSSHConsumerTest(TestCase):

    def test_get_target_user_from_query_string_valid(self):
        consumer = object.__new__(HostWebSSHConsumer)
        consumer.scope = cast(Any, {'query_string': b'token=abc&target_user=ubuntu'})
        self.assertEqual(consumer._get_target_user_from_query_string(), 'ubuntu')

    def test_get_target_user_from_query_string_invalid(self):
        consumer = object.__new__(HostWebSSHConsumer)
        consumer.scope = cast(Any, {'query_string': b'token=abc&target_user=../root'})
        self.assertEqual(consumer._get_target_user_from_query_string(), '')

    def test_extract_token_expire_at_from_payload(self):
        consumer = object.__new__(HostWebSSHConsumer)
        expire_at = consumer._extract_token_expire_at({'exp': 1893456000})
        self.assertEqual(expire_at, 1893456000.0)

    def test_extract_token_expire_at_invalid_value(self):
        consumer = object.__new__(HostWebSSHConsumer)
        self.assertIsNone(consumer._extract_token_expire_at({'exp': 'not-a-number'}))
        self.assertIsNone(consumer._extract_token_expire_at({}))

    def test_receive_should_close_when_token_expired(self):
        consumer = object.__new__(HostWebSSHConsumer)
        consumer.token_expire_at = 1.0
        consumer.audit_close_notified = False
        consumer._send_event = AsyncMock(return_value=None)
        consumer.close = AsyncMock(return_value=None)

        with patch('assets.consumers.time.time', return_value=2.0):
            import asyncio
            asyncio.run(consumer.receive(text_data='{"type":"ping"}'))

        consumer._send_event.assert_awaited_once_with('closed', {'message': '登录已过期，请重新登录后再连接'})
        consumer.close.assert_awaited_once_with(code=4401)

    def test_token_expiry_watchdog_should_close_when_token_expired(self):
        consumer = object.__new__(HostWebSSHConsumer)
        consumer.connected = True
        consumer.token_expire_at = 1.0
        consumer.audit_close_notified = False
        consumer._send_event = AsyncMock(return_value=None)
        consumer.close = AsyncMock(return_value=None)

        with patch('assets.consumers.time.time', return_value=2.0):
            import asyncio
            asyncio.run(consumer._token_expiry_watchdog())

        consumer._send_event.assert_awaited_once_with('closed', {'message': '登录已过期，请重新登录后再连接'})
        consumer.close.assert_awaited_once_with(code=4401)

    def test_token_expiry_watchdog_should_not_close_before_expiry(self):
        consumer = object.__new__(HostWebSSHConsumer)
        consumer.connected = True
        consumer.token_expire_at = 100.0
        consumer.audit_close_notified = False
        consumer._send_event = AsyncMock(return_value=None)
        consumer.close = AsyncMock(return_value=None)

        async def _break_loop(_seconds):
            consumer.connected = False
            return None

        with patch('assets.consumers.time.time', return_value=50.0), \
             patch('assets.consumers.asyncio.sleep', side_effect=_break_loop):
            import asyncio
            asyncio.run(consumer._token_expiry_watchdog())

        consumer._send_event.assert_not_awaited()
        consumer.close.assert_not_awaited()

