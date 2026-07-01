from django.test import TestCase
from rest_framework.test import APIClient
from rest_framework_jwt.settings import api_settings
from .models import Credential, Application, HostGroup, Host
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

    def test_list_hosts(self):
        """主机列表返回分页格式"""
        Host.objects.create(name='host1', ip='192.168.1.1', port=22)
        res = self.client.get('/assets/hosts/?page=1&page_size=10')
        body = self.assertResponseOK(res)
        self.assertIn('results', body['data'])
        self.assertIn('count', body['data'])

    def test_create_host(self):
        """新增主机"""
        group = HostGroup.objects.create(name='hg')
        res = self.client.post('/assets/hosts/', {
            'name': 'new_host',
            'ip': '192.168.1.100',
            'port': 22,
            'group_id': group.id,  # type: ignore[attr-defined]
        }, format='json')
        self.assertResponseOK(res)
        self.assertTrue(Host.objects.filter(name='new_host').exists())

    def test_get_host_detail(self):
        """获取主机详情"""
        host = Host.objects.create(name='detail_host', ip='192.168.1.1', port=22)
        res = self.client.get(f'/assets/hosts/{host.id}/')  # type: ignore[attr-defined]
        body = self.assertResponseOK(res)
        self.assertEqual(body['data']['name'], 'detail_host')

    def test_update_host(self):
        """编辑主机后返回完整主机信息"""
        host = Host.objects.create(name='old_host', ip='192.168.1.1', port=22)
        res = self.client.patch(f'/assets/hosts/{host.id}/', {  # type: ignore[attr-defined]
            'name': 'renamed_host'
        }, format='json')
        body = self.assertResponseOK(res)
        self.assertIn('id', body['data'])
        self.assertEqual(body['data']['name'], 'renamed_host')
        host.refresh_from_db()
        self.assertEqual(host.name, 'renamed_host')


# ─────────────────────────────────────────────
# 主机信息采集
# ─────────────────────────────────────────────
class HostCollectTest(BaseTestCase):

    def test_collect_host_without_credential(self):
        """采集主机时没有配置凭证应该返回 failed 状态"""
        host = Host.objects.create(name='test_host', ip='192.168.1.100', port=22)
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
        # 创建凭证和主机
        cred = Credential.objects.create(
            name='wrong_cred',
            username='root',
            password='wrong_password',
            auth_type=1,  # PASSWORD
            port=22
        )
        host = Host.objects.create(name='test_host', ip='192.168.1.100', port=22)
        from .models import HostCredential
        HostCredential.objects.create(host=host, credential=cred, is_default=True)
        
        res = self.client.post(f'/assets/hosts/{host.id}/collect-info/')  # type: ignore[attr-defined]
        body = self.assertResponseOK(res)
        # 应该返回 failed 状态
        self.assertEqual(body['data']['status'], 'failed')
        self.assertIn('error', body['data'])

    def test_collect_persists_failed_status_on_host(self):
        """采集失败（无凭证）应把 collect_status 持久化为 failed，并写入原因和时间"""
        from .tasks import collect_host_info
        host = Host.objects.create(name='persist_fail_host', ip='10.0.0.1', port=22)
        with self.assertRaises(Exception):
            collect_host_info(host)
        host.refresh_from_db()
        self.assertEqual(host.collect_status, 'failed')
        self.assertTrue(host.collect_message, msg='失败原因 collect_message 不应为空')
        self.assertIsNotNone(host.collect_time, msg='collect_time 应被写入')

    def test_scheduled_batch_collect_updates_status(self):
        """定时任务批量采集 collect_all_hosts_info 应更新每台主机的 collect_status"""
        from .tasks import collect_all_hosts_info
        host = Host.objects.create(name='batch_fail_host', ip='10.0.0.2', port=22)
        result = collect_all_hosts_info()
        self.assertTrue(result)
        host.refresh_from_db()
        self.assertEqual(host.collect_status, 'failed')
        self.assertTrue(host.collect_message, msg='批量采集失败也应写入 collect_message')
        self.assertIsNotNone(host.collect_time)

    def test_collect_persists_success_status_on_host(self):
        """采集成功应把 collect_status 持久化为 success 并清空失败原因"""
        from unittest.mock import patch
        from .tasks import collect_host_info
        from .models import HostCredential
        cred = Credential.objects.create(
            name='ok_cred', username='root', password='pw', auth_type=1, port=22
        )
        host = Host.objects.create(name='ok_host', ip='10.0.0.3', port=22)
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
