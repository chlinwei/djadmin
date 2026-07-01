from django.test import TestCase
from rest_framework.test import APIClient
from rest_framework_jwt.settings import api_settings
from .models import SysRole
from user.models import SysUser, SysUserRole


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
        body = res.json()
        self.assertEqual(body.get('code'), 200, msg=f"Expected code=200, got: {body}")
        return body


# ─────────────────────────────────────────────
# 角色管理
# ─────────────────────────────────────────────
class RoleManageTest(BaseTestCase):

    def test_list_roles(self):
        """角色列表返回分页格式"""
        SysRole.objects.create(name='管理员', code='admin')
        res = self.client.get('/sys/roles/?page=1&page_size=10')
        body = self.assertResponseOK(res)
        self.assertIn('results', body['data'])
        self.assertIn('count', body['data'])

    def test_create_role(self):
        """新增角色"""
        res = self.client.post('/sys/roles/', {
            'name': '测试角色', 'code': 'test_role', 'remark': '测试',
        }, format='json')
        self.assertResponseOK(res)
        self.assertTrue(SysRole.objects.filter(code='test_role').exists())

    def test_get_role_detail(self):
        """获取角色详情"""
        role = SysRole.objects.create(name='角色1', code='role1')
        res = self.client.get(f'/sys/roles/{role.id}/')
        body = self.assertResponseOK(res)
        self.assertEqual(body['data']['code'], 'role1')

    def test_update_role(self):
        """编辑角色"""
        role = SysRole.objects.create(name='旧名称', code='old_code')
        res = self.client.patch(f'/sys/roles/{role.id}/', {
            'name': '新名称', 'code': 'old_code',
        }, format='json')
        self.assertResponseOK(res)
        role.refresh_from_db()
        self.assertEqual(role.name, '新名称')

    def test_batch_delete_roles(self):
        """批量删除角色"""
        role = SysRole.objects.create(name='待删除', code='to_delete')
        res = self.client.delete('/sys/roles/batch-delete/', {
            'role_ids': [role.id]
        }, format='json')
        self.assertResponseOK(res)
        self.assertFalse(SysRole.objects.filter(id=role.id).exists())

    def test_batch_delete_empty_ids(self):
        """批量删除不传 ids 应报错"""
        res = self.client.delete('/sys/roles/batch-delete/', {'role_ids': []}, format='json')
        self.assertNotEqual(res.json()['code'], 200)

    def test_batch_delete_cascades_user_role(self):
        """删除角色时级联删除用户-角色关联"""
        role = SysRole.objects.create(name='有绑定', code='bound')
        SysUserRole.objects.create(user=self.user, role=role)
        self.client.delete('/sys/roles/batch-delete/', {'role_ids': [role.id]}, format='json')
        self.assertFalse(SysUserRole.objects.filter(role_id=role.id).exists())

    def test_get_current_user_role_list(self):
        """获取当前用户角色列表"""
        role = SysRole.objects.create(name='我的角色', code='my_role')
        SysUserRole.objects.create(user=self.user, role=role)
        res = self.client.get('/sys/roles/getCurrentUserRoleList/')
        body = self.assertResponseOK(res)
        self.assertIn('roleList', body['data'])
        self.assertEqual(len(body['data']['roleList']), 1)
