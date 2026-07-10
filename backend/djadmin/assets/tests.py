from django.test import TestCase
from django.core.files.uploadedfile import SimpleUploadedFile
from unittest.mock import AsyncMock, MagicMock, patch
from contextlib import contextmanager
from rest_framework.test import APIClient
from rest_framework_jwt.settings import api_settings
from .models import Credential, Application, HostGroup, Host, HostCredential, HostDisk, HostHardware, WebSSHSessionLog
from .consumers import HostWebSSHConsumer
from .webssh_runtime import WebSSHRuntimeRegistry
from user.models import SysUser


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
        host = Host.objects.create(instance_name='cred_host', ip='192.168.1.120', port=22)
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
        host = Host.objects.create(instance_name='cred_host_keep', ip='192.168.1.121', port=22)
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
        Host.objects.create(instance_name='host1', ip='192.168.1.1', port=22)
        res = self.client.get('/assets/hosts/?page=1&page_size=10')
        body = self.assertResponseOK(res)
        self.assertIn('results', body['data'])
        self.assertIn('count', body['data'])

    def test_list_hosts_with_host_id_filter(self):
        """按主机 ID 过滤应精确返回单台主机"""
        target = Host.objects.create(instance_name='test3', ip='192.168.1.10', port=22)
        Host.objects.create(instance_name='test3-bak', ip='192.168.1.11', port=22)
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
            'port': 22,
            'group_id': group.id,  # type: ignore[attr-defined]
        }, format='json')
        self.assertResponseOK(res)
        self.assertTrue(Host.objects.filter(instance_name='new_host').exists())

    def test_get_host_detail(self):
        """获取主机详情"""
        host = Host.objects.create(instance_name='detail_host', ip='192.168.1.1', port=22)
        res = self.client.get(f'/assets/hosts/{host.id}/')  # type: ignore[attr-defined]
        body = self.assertResponseOK(res)
        self.assertEqual(body['data']['instance_name'], 'detail_host')

    def test_get_host_detail_ignores_sr0_in_disks(self):
        """主机详情应隐藏 /dev/sr0 磁盘，且使用率仅按有效磁盘计算。"""
        host = Host.objects.create(instance_name='detail_disk_host', ip='192.168.1.20', port=22)
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

        res = self.client.get(f'/assets/hosts/{host.id}/')  # type: ignore[attr-defined]
        body = self.assertResponseOK(res)
        self.assertEqual(len(body['data']['disks']), 1)
        self.assertEqual(body['data']['disks'][0]['device'], '/dev/sda1')
        self.assertEqual(body['data']['disk_used_percent'], 40.0)

    def test_update_host(self):
        """编辑主机后返回完整主机信息"""
        host = Host.objects.create(instance_name='old_host', ip='192.168.1.1', port=22)
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
        host = Host.objects.create(instance_name='ws_host_del', ip='192.168.1.200', port=22)
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
        host_a = Host.objects.create(instance_name='ws_batch_a', ip='192.168.2.10', port=22)
        host_b = Host.objects.create(instance_name='ws_batch_b', ip='192.168.2.11', port=22)
        host_c = Host.objects.create(instance_name='ws_batch_c', ip='192.168.2.12', port=22)

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
        host = Host.objects.create(instance_name='ws_host_ip', ip='192.168.1.210', port=22)
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

    def test_update_host_port_should_force_close_active_webssh_sessions(self):
        """修改主机 SSH 端口后，应主动断开该主机在线 WebSSH 会话。"""
        host = Host.objects.create(instance_name='ws_host_port', ip='192.168.1.220', port=22)
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
                {'port': 2222},
                format='json',
            )
            self.assertResponseOK(res)
            fake_consumer.close.assert_awaited_once()
            self.assertEqual(WebSSHRuntimeRegistry.get_active_count_for_host(host.id), 0)
        finally:
            WebSSHRuntimeRegistry.mark_inactive(session.id)  # type: ignore[arg-type]

    def test_update_host_default_credential_should_force_close_active_webssh_sessions(self):
        """切换主机默认凭证后，应主动断开该主机在线 WebSSH 会话。"""
        credential_a = Credential.objects.create(
            name='cred-a',
            username='root',
            password='secret-a',
            port=22,
            auth_type=Credential.AuthType.PASSWORD,
        )
        credential_b = Credential.objects.create(
            name='cred-b',
            username='root',
            password='secret-b',
            port=22,
            auth_type=Credential.AuthType.PASSWORD,
        )
        host = Host.objects.create(instance_name='ws_host_cred', ip='192.168.1.230', port=22)
        HostCredential.objects.create(host=host, credential=credential_a, is_default=True)
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
                {'credential_id': credential_b.id},  # type: ignore[attr-defined]
                format='json',
            )
            self.assertResponseOK(res)
            fake_consumer.close.assert_awaited_once()
            self.assertEqual(WebSSHRuntimeRegistry.get_active_count_for_host(host.id), 0)
        finally:
            WebSSHRuntimeRegistry.mark_inactive(session.id)  # type: ignore[arg-type]

    def test_update_host_non_connection_fields_should_not_force_close_active_webssh_sessions(self):
        """仅修改主机展示字段（如名称）时，不应误断开在线 WebSSH 会话。"""
        host = Host.objects.create(instance_name='ws_host_no_close', ip='192.168.1.240', port=22)
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
        host = Host.objects.create(instance_name='ws_host', ip='192.168.1.2', port=22)
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
        host_a = Host.objects.create(instance_name='ws_host_a', ip='192.168.1.21', port=22)
        host_b = Host.objects.create(instance_name='ws_host_b', ip='192.168.1.22', port=22)
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
        host = Host.objects.create(instance_name='ws_host_c', ip='192.168.1.23', port=22)
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
        host = Host.objects.create(instance_name='ws_host_files', ip='192.168.1.24', port=22)

        class _Attr:
            def __init__(self, filename, st_mode, st_size, st_mtime):
                self.filename = filename
                self.st_mode = st_mode
                self.st_size = st_size
                self.st_mtime = st_mtime

        fake_sftp = MagicMock()
        fake_sftp.normalize.return_value = '/root'
        fake_sftp.listdir_attr.return_value = [
            _Attr('b.txt', 0o100644, 20, 1710000000),
            _Attr('apps', 0o040755, 0, 1710000100),
        ]

        with self._active_webssh_session(host):
            with patch('assets.views.HostManage._connect_sftp', return_value=(MagicMock(), fake_sftp)), \
                 patch('assets.views.HostManage._close_sftp', return_value=None):
                res = self.client.get(f'/assets/hosts/{host.id}/files/list/?path=/root')  # type: ignore[attr-defined]
                body = self.assertResponseOK(res)
                self.assertEqual(body['data']['current_path'], '/root')
                self.assertEqual(body['data']['entries'][0]['name'], 'apps')
                self.assertTrue(body['data']['entries'][0]['is_dir'])
                self.assertEqual(body['data']['entries'][1]['name'], 'b.txt')

    def test_rename_webssh_file(self):
        """可重命名主机文件。"""
        host = Host.objects.create(instance_name='ws_host_rename', ip='192.168.1.25', port=22)
        fake_sftp = MagicMock()
        fake_sftp.normalize.return_value = '/root/old.txt'
        with self._active_webssh_session(host):
            with patch('assets.views.HostManage._connect_sftp', return_value=(MagicMock(), fake_sftp)), \
                 patch('assets.views.HostManage._close_sftp', return_value=None):
                res = self.client.post(
                    f'/assets/hosts/{host.id}/files/rename/',  # type: ignore[attr-defined]
                    {'path': '/root/old.txt', 'new_name': 'new.txt'},
                    format='json',
                )
                body = self.assertResponseOK(res)
                self.assertEqual(body['data']['name'], 'new.txt')
                fake_sftp.rename.assert_called_once_with('/root/old.txt', '/root/new.txt')

    def test_download_webssh_file_streaming(self):
        """下载主机文件应使用流式响应，避免大文件内存占用。"""
        host = Host.objects.create(instance_name='ws_host_download', ip='192.168.1.28', port=22)
        fake_ssh = MagicMock()
        fake_sftp = MagicMock()
        fake_sftp.normalize.return_value = '/root/big.bin'
        fake_sftp.lstat.return_value = type('Stat', (), {'st_mode': 0o100644, 'st_size': 6})()
        fake_remote_file = MagicMock()
        fake_remote_file.read.side_effect = [b'abc', b'def', b'']
        fake_sftp.file.return_value = fake_remote_file

        with self._active_webssh_session(host):
            with patch('assets.views.HostManage._connect_sftp', return_value=(fake_ssh, fake_sftp)), \
                 patch('assets.views.HostManage._close_sftp', return_value=None) as mock_close:
                res = self.client.get(f'/assets/hosts/{host.id}/files/download/?path=/root/big.bin')  # type: ignore[attr-defined]
                self.assertEqual(res.status_code, 200)
                self.assertEqual(res.get('Content-Length'), '6')
                self.assertIn("attachment; filename*=UTF-8''big.bin", res.get('Content-Disposition'))
                self.assertEqual(b''.join(res.streaming_content), b'abcdef')
                fake_sftp.file.assert_called_once_with('/root/big.bin', 'rb')
                fake_remote_file.close.assert_called_once()
                mock_close.assert_called_once_with(fake_ssh, fake_sftp)

    def test_download_webssh_file_with_range(self):
        """下载主机文件支持 Range 断点续传。"""
        host = Host.objects.create(instance_name='ws_host_range', ip='192.168.1.29', port=22)
        fake_ssh = MagicMock()
        fake_sftp = MagicMock()
        fake_sftp.normalize.return_value = '/root/range.bin'
        fake_sftp.lstat.return_value = type('Stat', (), {'st_mode': 0o100644, 'st_size': 10})()
        fake_remote_file = MagicMock()
        fake_remote_file.read.side_effect = [b'3456', b'']
        fake_sftp.file.return_value = fake_remote_file

        with self._active_webssh_session(host):
            with patch('assets.views.HostManage._connect_sftp', return_value=(fake_ssh, fake_sftp)), \
                 patch('assets.views.HostManage._close_sftp', return_value=None) as mock_close:
                res = self.client.get(
                    f'/assets/hosts/{host.id}/files/download/?path=/root/range.bin',  # type: ignore[attr-defined]
                    HTTP_RANGE='bytes=3-6',
                )
                self.assertEqual(res.status_code, 206)
                self.assertEqual(res.get('Content-Range'), 'bytes 3-6/10')
                self.assertEqual(res.get('Content-Length'), '4')
                self.assertEqual(b''.join(res.streaming_content), b'3456')
                fake_remote_file.seek.assert_called_once_with(3)
                fake_remote_file.close.assert_called_once()
                mock_close.assert_called_once_with(fake_ssh, fake_sftp)

    def test_issue_webssh_file_download_ticket(self):
        """可签发独立传输服务下载票据。"""
        host = Host.objects.create(instance_name='ws_host_ticket', ip='192.168.1.30', port=22)
        with self._active_webssh_session(host):
            with patch('assets.views.issue_download_ticket', return_value='ticket-123') as mock_issue_ticket:
                res = self.client.post(
                    f'/assets/hosts/{host.id}/files/download-ticket/',  # type: ignore[attr-defined]
                    {'path': '/root/file.bin'},
                    format='json',
                )
                body = self.assertResponseOK(res)
                self.assertEqual(body['data']['ticket'], 'ticket-123')
                self.assertIn('/transfer/download/?ticket=ticket-123', body['data']['download_url'])
                self.assertGreater(body['data']['expires_in'], 0)
                mock_issue_ticket.assert_called_once()

    def test_issue_webssh_file_upload_ticket(self):
        """可签发独立传输服务上传票据。"""
        host = Host.objects.create(instance_name='ws_host_upload_ticket', ip='192.168.1.31', port=22)
        with self._active_webssh_session(host):
            with patch('assets.views.issue_upload_ticket', return_value='ticket-up-123') as mock_issue_ticket:
                res = self.client.post(
                    f'/assets/hosts/{host.id}/files/upload-ticket/',  # type: ignore[attr-defined]
                    {'path': '/root', 'filename': 'part.bin'},
                    format='json',
                )
                body = self.assertResponseOK(res)
                self.assertEqual(body['data']['ticket'], 'ticket-up-123')
                self.assertTrue(body['data']['upload_chunk_url'].endswith('/transfer/upload/chunk/'))
                self.assertTrue(body['data']['upload_status_url'].endswith('/transfer/upload/status/'))
                self.assertTrue(body['data']['upload_cancel_url'].endswith('/transfer/upload/cancel/'))
                self.assertGreater(body['data']['expires_in'], 0)
                mock_issue_ticket.assert_called_once()

    def test_create_webssh_directory(self):
        """可创建远端目录。"""
        host = Host.objects.create(instance_name='ws_host_mkdir', ip='192.168.1.26', port=22)
        fake_sftp = MagicMock()
        fake_sftp.normalize.return_value = '/root'
        with self._active_webssh_session(host):
            with patch('assets.views.HostManage._connect_sftp', return_value=(MagicMock(), fake_sftp)), \
                 patch('assets.views.HostManage._close_sftp', return_value=None):
                res = self.client.post(
                    f'/assets/hosts/{host.id}/files/create-dir/',  # type: ignore[attr-defined]
                    {'path': '/root', 'name': 'logs'},
                    format='json',
                )
                body = self.assertResponseOK(res)
                self.assertEqual(body['data']['path'], '/root/logs')
                fake_sftp.mkdir.assert_called_once_with('/root/logs')

    def test_upload_webssh_file_by_chunks(self):
        """分片上传完成后应合并为最终文件。"""
        host = Host.objects.create(instance_name='ws_host_upload_chunk', ip='192.168.1.30', port=22)
        fake_sftp = MagicMock()
        fake_sftp.normalize.return_value = '/root'
        fake_file_cm = MagicMock()
        fake_sftp.file.return_value = fake_file_cm
        fake_remote_file = MagicMock()
        fake_file_cm.__enter__.return_value = fake_remote_file
        fake_file_cm.__exit__.return_value = False
        fake_sftp.lstat.side_effect = Exception('not exists')

        with self._active_webssh_session(host):
            with patch('assets.views.HostManage._connect_sftp', return_value=(MagicMock(), fake_sftp)), \
                 patch('assets.views.HostManage._close_sftp', return_value=None):
                res1 = self.client.post(
                    f'/assets/hosts/{host.id}/files/upload/chunk/',  # type: ignore[attr-defined]
                    {
                        'path': '/root',
                        'filename': 'part.bin',
                        'upload_id': 'u123',
                        'chunk_index': '0',
                        'total_chunks': '2',
                        'chunk': SimpleUploadedFile('chunk0', b'abc'),
                    },
                    format='multipart',
                )
                body1 = self.assertResponseOK(res1)
                self.assertFalse(body1['data']['done'])

                res2 = self.client.post(
                    f'/assets/hosts/{host.id}/files/upload/chunk/',  # type: ignore[attr-defined]
                    {
                        'path': '/root',
                        'filename': 'part.bin',
                        'upload_id': 'u123',
                        'chunk_index': '1',
                        'total_chunks': '2',
                        'chunk': SimpleUploadedFile('chunk1', b'def'),
                    },
                    format='multipart',
                )
                body2 = self.assertResponseOK(res2)
                self.assertTrue(body2['data']['done'])
                self.assertEqual(body2['data']['path'], '/root/part.bin')

                self.assertEqual(fake_sftp.file.call_args_list[0].args, ('/root/.part.bin.u123.part', 'wb'))
                self.assertEqual(fake_sftp.file.call_args_list[1].args, ('/root/.part.bin.u123.part', 'ab'))
                fake_remote_file.write.assert_any_call(b'abc')
                fake_remote_file.write.assert_any_call(b'def')
                fake_sftp.rename.assert_called_once_with('/root/.part.bin.u123.part', '/root/part.bin')

    def test_cancel_webssh_chunk_upload(self):
        """取消分片上传应删除远端临时分片文件。"""
        host = Host.objects.create(instance_name='ws_host_upload_cancel', ip='192.168.1.31', port=22)
        fake_sftp = MagicMock()
        fake_sftp.normalize.return_value = '/root'

        with self._active_webssh_session(host):
            with patch('assets.views.HostManage._connect_sftp', return_value=(MagicMock(), fake_sftp)), \
                 patch('assets.views.HostManage._close_sftp', return_value=None):
                res = self.client.post(
                    f'/assets/hosts/{host.id}/files/upload/cancel/',  # type: ignore[attr-defined]
                    {'path': '/root', 'filename': 'part.bin', 'upload_id': 'u123'},
                    format='json',
                )
                body = self.assertResponseOK(res)
                self.assertTrue(body['data']['canceled'])
                fake_sftp.remove.assert_called_once_with('/root/.part.bin.u123.part')

    def test_get_webssh_chunk_upload_status(self):
        """查询分片上传状态应返回当前已上传字节和下一分片下标。"""
        host = Host.objects.create(instance_name='ws_host_upload_status', ip='192.168.1.32', port=22)
        fake_sftp = MagicMock()
        fake_sftp.normalize.return_value = '/root'
        fake_sftp.lstat.return_value = type('Stat', (), {'st_mode': 0o100644, 'st_size': 16 * 1024 * 1024})()

        with self._active_webssh_session(host):
            with patch('assets.views.HostManage._connect_sftp', return_value=(MagicMock(), fake_sftp)), \
                 patch('assets.views.HostManage._close_sftp', return_value=None):
                res = self.client.get(
                    f'/assets/hosts/{host.id}/files/upload/status/?path=/root&filename=part.bin&upload_id=u123&chunk_size=8388608'  # type: ignore[attr-defined]
                )
                body = self.assertResponseOK(res)
                self.assertTrue(body['data']['exists'])
                self.assertEqual(body['data']['uploaded_size'], 16 * 1024 * 1024)
                self.assertEqual(body['data']['next_chunk_index'], 2)

    def test_get_webssh_chunk_upload_status_not_exists(self):
        """上传状态查询在临时分片不存在时应返回 exists=False。"""
        host = Host.objects.create(instance_name='ws_host_upload_status_empty', ip='192.168.1.33', port=22)
        fake_sftp = MagicMock()
        fake_sftp.normalize.return_value = '/root'
        fake_sftp.lstat.side_effect = Exception('not exists')

        with self._active_webssh_session(host):
            with patch('assets.views.HostManage._connect_sftp', return_value=(MagicMock(), fake_sftp)), \
                 patch('assets.views.HostManage._close_sftp', return_value=None):
                res = self.client.get(
                    f'/assets/hosts/{host.id}/files/upload/status/?path=/root&filename=part.bin&upload_id=u123&chunk_size=8388608'  # type: ignore[attr-defined]
                )
                body = self.assertResponseOK(res)
                self.assertFalse(body['data']['exists'])
                self.assertEqual(body['data']['uploaded_size'], 0)
                self.assertEqual(body['data']['next_chunk_index'], 0)

    def test_create_webssh_empty_file(self):
        """可创建远端空文件。"""
        host = Host.objects.create(instance_name='ws_host_touch', ip='192.168.1.27', port=22)
        fake_sftp = MagicMock()
        fake_sftp.normalize.return_value = '/root'
        fake_remote_file = MagicMock()
        fake_sftp.file.return_value.__enter__.return_value = fake_remote_file
        fake_sftp.file.return_value.__exit__.return_value = False
        with self._active_webssh_session(host):
            with patch('assets.views.HostManage._connect_sftp', return_value=(MagicMock(), fake_sftp)), \
                 patch('assets.views.HostManage._close_sftp', return_value=None):
                res = self.client.post(
                    f'/assets/hosts/{host.id}/files/create-file/',  # type: ignore[attr-defined]
                    {'path': '/root', 'name': 'empty.txt'},
                    format='json',
                )
                body = self.assertResponseOK(res)
                self.assertEqual(body['data']['path'], '/root/empty.txt')
                fake_sftp.file.assert_called_once_with('/root/empty.txt', 'xb')
                fake_remote_file.write.assert_called_once_with(b'')


# ─────────────────────────────────────────────
# 主机信息采集
# ─────────────────────────────────────────────
class HostCollectTest(BaseTestCase):

    def test_collect_host_without_credential(self):
        """采集主机时没有配置凭证应该返回 failed 状态"""
        host = Host.objects.create(instance_name='test_host', ip='192.168.1.100', port=22)
        res = self.client.post(f'/assets/hosts/{host.id}/collect-info/')  # type: ignore[attr-defined]
        body = self.assertResponseOK(res)
        # 单个采集应该返回 collected 或 failed 状态
        self.assertIn('status', body['data'])
        self.assertIn(body['data']['status'], ['collected', 'failed'])
        # 没有凭证应该返回 failed
        self.assertEqual(body['data']['status'], 'failed')
        self.assertIn('error', body['data'])
        self.assertIn('凭证', body['data']['message'])

    def test_collect_host_with_invalid_connection(self):
        """采集主机时凭证错误应该返回 failed 状态"""
        cred = Credential.objects.create(
            name='wrong_cred',
            username='root',
            password='wrong_password',
            auth_type=1,  # PASSWORD
            port=22
        )
        host = Host.objects.create(instance_name='test_host', ip='192.168.1.100', port=22)
        from .models import HostCredential
        HostCredential.objects.create(host=host, credential=cred, is_default=True)

        # 直接让 SSH 连接立即抛出认证失败，跳过真实网络 TCP 超时（原本约 30s）
        with patch('assets.tasks._run_ssh_command', side_effect=Exception('Authentication failed.')):
            res = self.client.post(f'/assets/hosts/{host.id}/collect-info/')  # type: ignore[attr-defined]
        body = self.assertResponseOK(res)
        self.assertEqual(body['data']['status'], 'failed')
        self.assertIn('error', body['data'])

    def test_collect_persists_failed_status_on_host(self):
        """采集失败（无凭证）应把 collect_status 持久化为 failed，并写入原因和时间"""
        from .tasks import collect_host_info
        host = Host.objects.create(instance_name='persist_fail_host', ip='10.0.0.1', port=22)
        with self.assertRaises(Exception):
            collect_host_info(host)
        host.refresh_from_db()
        self.assertEqual(host.collect_status, 'failed')
        self.assertTrue(host.collect_message, msg='失败原因 collect_message 不应为空')
        self.assertIsNotNone(host.collect_time, msg='collect_time 应被写入')

    def test_scheduled_batch_collect_updates_status(self):
        """定时任务批量采集 collect_all_hosts_info 应更新每台主机的 collect_status"""
        from .tasks import collect_all_hosts_info
        host = Host.objects.create(instance_name='batch_fail_host', ip='10.0.0.2', port=22)
        result = collect_all_hosts_info()
        self.assertTrue(result)
        host.refresh_from_db()
        self.assertEqual(host.collect_status, 'unknown')
        self.assertIn('定时采集已跳过', host.collect_message)
        self.assertIsNotNone(host.collect_time)

    def test_collect_auth_failure_reaches_lock_threshold(self):
        """连续认证失败达到阈值后，应触发自动采集保护期。"""
        from .tasks import collect_host_info
        from .models import HostCredential

        cred = Credential.objects.create(
            name='bad_auth_cred', username='root', password='wrong', auth_type=1, port=22
        )
        host = Host.objects.create(instance_name='auth_lock_host', ip='10.0.0.8', port=22)
        HostCredential.objects.create(host=host, credential=cred, is_default=True)

        with patch('assets.tasks._collect_linux_info', side_effect=ValueError('SSH 认证失败：Authentication failed.')):
            for _ in range(3):
                with self.assertRaises(Exception):
                    collect_host_info(host)

        host.refresh_from_db()
        self.assertGreaterEqual(host.auth_failed_count, 3)
        self.assertIsNotNone(host.last_auth_failed_time)
        self.assertIsNotNone(host.auth_lock_until)
        self.assertIn('连续认证失败已达到', host.collect_message)

    def test_scheduled_collect_skips_locked_host(self):
        """定时任务应跳过处于认证保护期的主机，避免持续触发账号锁定。"""
        from django.utils import timezone
        from datetime import timedelta
        from .tasks import collect_all_hosts_info

        host = Host.objects.create(
            instance_name='locked_host',
            ip='10.0.0.9',
            port=22,
            auth_failed_count=3,
            auth_lock_until=timezone.now() + timedelta(minutes=15),
        )

        with patch('assets.tasks.collect_host_info') as mocked_collect:
            result = collect_all_hosts_info()

        self.assertTrue(result)
        mocked_collect.assert_not_called()
        host.refresh_from_db()
        self.assertEqual(host.collect_status, 'unknown')
        self.assertIn('处于保护期', host.collect_message)


# ─────────────────────────────────────────────
# WebSSH Consumer
# ─────────────────────────────────────────────
class HostWebSSHConsumerTest(TestCase):

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

    def test_collect_persists_success_status_on_host(self):
        """采集成功应把 collect_status 持久化为 success 并清空失败原因"""
        from unittest.mock import patch
        from .tasks import collect_host_info
        from .models import HostCredential
        cred = Credential.objects.create(
            name='ok_cred', username='root', password='pw', auth_type=1, port=22
        )
        host = Host.objects.create(instance_name='ok_host', ip='10.0.0.3', port=22)
        HostCredential.objects.create(host=host, credential=cred, is_default=True)
        fake_data = {
            'system': {
                'os_type': 'Linux', 'os_version': 'Ubuntu 22.04',
                'kernel_version': '5.15.0', 'hostname': 'okh', 'agent_version': '1.0',
            },
            'hardware': {
                'cpu_cores': 4, 'cpu_model': 'Intel', 'memory_gb': 8,
                'disk_total_gb': 100, 'architecture': 'x86_64',
            },
            'disks': [],
        }
        with patch('assets.tasks._collect_linux_info', return_value=fake_data):
            collect_host_info(host)
        host.refresh_from_db()
        self.assertEqual(host.collect_status, 'success')
        self.assertEqual(host.collect_message, '')
        self.assertIsNotNone(host.collect_time)

    def test_collect_linux_info_excludes_optical_device_sr0(self):
        """磁盘采集应忽略 /dev/sr0，避免无意义光驱统计进入结果。"""
        from unittest.mock import patch
        from .tasks import _collect_linux_info

        command_outputs = [
            'NAME="Ubuntu"\nVERSION="22.04 LTS"',
            'host-a',
            '5.15.0',
            '4',
            'Intel(R) Xeon(R)',
            '8192',
            'x86_64',
            '/dev/sda1 / 100G 40G ext4\n/dev/sr0 /media/cdrom 1G 1G iso9660',
        ]

        with patch('assets.tasks._run_ssh_command', side_effect=command_outputs):
            data = _collect_linux_info(object(), object())

        self.assertEqual(len(data['disks']), 1)
        self.assertEqual(data['disks'][0]['device'], '/dev/sda1')
        self.assertEqual(data['hardware']['disk_total_gb'], 100.0)
