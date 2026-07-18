from django.test import TestCase
from django.contrib.auth.hashers import check_password, make_password
from django.utils import timezone
from datetime import timedelta
from datetime import timezone as dt_timezone
from rest_framework.test import APIClient
from rest_framework_jwt.settings import api_settings
from .models import ApiToken, SysUser, SysUserRole
from role.models import SysRole


def _get_token(user: SysUser) -> str:
    """直接生成 JWT token，不经过登录接口（避免原始 SQL 兼容问题）"""
    jwt_payload_handler = api_settings.JWT_PAYLOAD_HANDLER  # type: ignore[operator]
    jwt_encode_handler = api_settings.JWT_ENCODE_HANDLER  # type: ignore[operator]
    payload = jwt_payload_handler(user)  # type: ignore[operator]
    return jwt_encode_handler(payload)  # type: ignore[operator]


class BaseTestCase(TestCase):
    """所有测试的基类：创建测试用户并通过 JWT 直接鉴权"""

    def setUp(self):
        self.client = APIClient()
        self.user = SysUser.objects.create(
            username='admin',
            password=make_password('admin123'),
            status=1,
            email='admin@test.com',
            timezone='Asia/Shanghai',
        )
        token = _get_token(self.user)
        self.client.credentials(HTTP_AUTHORIZATION=token)

    def assertResponseOK(self, res):
        """断言响应 code==200"""
        body = res.json()
        self.assertEqual(body.get('code'), 200, msg=f"Expected code=200, got: {body}")
        return body

    def assertResponseCode(self, res, code: int):
        """断言响应是指定 code"""
        body = res.json()
        self.assertEqual(body.get('code'), code, msg=f"Expected code={code}, got: {body}")
        return body


# ─────────────────────────────────────────────
# 登录接口
# ─────────────────────────────────────────────
class LoginViewTest(TestCase):
    def setUp(self):
        self.client = APIClient()
        SysUser.objects.create(username='testuser', password=make_password('pass123'), status=1)
        SysUser.objects.create(username='disabled', password='pass123', status=0)

    def test_login_success(self):
        """正常登录返回 token"""
        res = self.client.post('/sys/login', {
            'username': 'testuser', 'password': 'pass123'
        }, format='json')
        body = res.json()
        self.assertEqual(body['code'], 200)
        self.assertIn('token', body['data'])
        self.assertIn('currentUser', body['data'])
        self.assertIn('menuList', body['data'])
        self.assertIn('role_codes', body['data'])

    def test_login_wrong_password(self):
        """密码错误返回 code=300"""
        res = self.client.post('/sys/login', {
            'username': 'testuser', 'password': 'wrongpassword'
        }, format='json')
        self.assertEqual(res.json()['code'], 300)

    def test_login_user_not_exist(self):
        """用户不存在返回 code=300"""
        res = self.client.post('/sys/login', {
            'username': 'nobody', 'password': 'pass123'
        }, format='json')
        self.assertEqual(res.json()['code'], 300)

    def test_login_disabled_user(self):
        """禁用用户不能登录"""
        res = self.client.post('/sys/login', {
            'username': 'disabled', 'password': 'pass123'
        }, format='json')
        self.assertNotEqual(res.json()['code'], 200)

    def test_login_response_format(self):
        """验证响应格式有 code/msg/data 三个字段"""
        res = self.client.post('/sys/login', {
            'username': 'testuser', 'password': 'pass123'
        }, format='json')
        body = res.json()
        self.assertIn('code', body)
        self.assertIn('msg', body)
        self.assertIn('data', body)


# ─────────────────────────────────────────────
# 用户管理接口
# ─────────────────────────────────────────────
class UserManageTest(BaseTestCase):

    def test_list_users_returns_paginated(self):
        """用户列表返回分页格式"""
        res = self.client.get('/sys/users/?page=1&page_size=10')
        body = self.assertResponseOK(res)
        data = body['data']
        self.assertIn('results', data)
        self.assertIn('count', data)
        self.assertIn('pageNumber', data)
        self.assertIn('pageSize', data)
        self.assertIn('totalPages', data)

    def test_list_users_without_token_returns_301(self):
        """无 token 访问返回 301"""
        client = APIClient()
        res = client.get('/sys/users/')
        self.assertEqual(res.json()['code'], 301)  # type: ignore[attr-defined]

    def test_get_user_detail(self):
        """获取用户详情"""
        res = self.client.get(f'/sys/users/{self.user.id}/')
        body = self.assertResponseOK(res)
        self.assertEqual(body['data']['username'], 'admin')

    def test_create_user(self):
        """新增用户"""
        res = self.client.post('/sys/users/', {
            'username': 'newuser',
            'email': 'new@test.com',
            'status': 1,
        }, format='json')
        self.assertResponseOK(res)
        self.assertTrue(SysUser.objects.filter(username='newuser').exists())

    def test_create_user_duplicate_username(self):
        """重复用户名应失败"""
        self.client.post('/sys/users/', {
            'username': 'dupuser', 'status': 1
        }, format='json')
        res = self.client.post('/sys/users/', {
            'username': 'dupuser', 'status': 1
        }, format='json')
        # 期望返回错误（code != 200）
        self.assertNotEqual(res.json()['code'], 200)

    def test_update_user(self):
        """编辑用户"""
        res = self.client.patch(f'/sys/users/{self.user.id}/', {
            'email': 'updated@test.com',
        }, format='json')
        self.assertResponseOK(res)
        self.user.refresh_from_db()
        self.assertEqual(self.user.email, 'updated@test.com')

    def test_batch_delete_users(self):
        """批量删除用户"""
        u = SysUser.objects.create(username='todelete', password='x', status=1)
        res = self.client.delete('/sys/users/userBatchDelete/', {
            'user_ids': [u.id]
        }, format='json')
        self.assertResponseOK(res)
        self.assertFalse(SysUser.objects.filter(id=u.id).exists())

    def test_batch_delete_empty_ids(self):
        """批量删除不传 ids 应报错"""
        res = self.client.delete('/sys/users/userBatchDelete/', {
            'user_ids': []
        }, format='json')
        self.assertNotEqual(res.json()['code'], 200)

    def test_reset_password(self):
        """重置用户密码为 123456"""
        u = SysUser.objects.create(username='resetme', password=make_password('oldpass'), status=1)
        res = self.client.post('/sys/users/resetUserPwd/', {
            'id': u.id
        }, format='json')
        self.assertResponseOK(res)
        u.refresh_from_db()
        self.assertTrue(check_password('123456', u.password))

    def test_change_user_status(self):
        """修改用户状态为禁用"""
        res = self.client.post('/sys/users/changeUserStatus/', {
            'user_id': self.user.id,
            'status': 0,
        }, format='json')
        self.assertResponseOK(res)
        self.user.refresh_from_db()
        self.assertEqual(self.user.status, 0)

    def test_check_username_exists(self):
        """检查用户名存在"""
        res = self.client.get('/sys/users/checkUserName/?username=admin')
        body = self.assertResponseOK(res)
        self.assertTrue(body['data']['exists'])

    def test_check_username_not_exists(self):
        """检查用户名不存在"""
        res = self.client.get('/sys/users/checkUserName/?username=nobody123')
        body = self.assertResponseOK(res)
        self.assertFalse(body['data']['exists'])

    def test_assign_roles(self):
        """分配用户角色"""
        role = SysRole.objects.create(name='测试角色', code='test_role')
        res = self.client.post('/sys/users/assginUserRoles/', {
            'user_id': self.user.id,
            'roleIds': [role.id],
        }, format='json')
        self.assertResponseOK(res)
        self.assertTrue(
            SysUserRole.objects.filter(user_id=self.user.id, role_id=role.id).exists()
        )

    def test_get_user_roles(self):
        """获取用户角色列表"""
        role = SysRole.objects.create(name='角色A', code='role_a')
        SysUserRole.objects.create(user=self.user, role=role)
        res = self.client.get(f'/sys/users/getUserRolesById/?user_id={self.user.id}')
        body = self.assertResponseOK(res)
        self.assertIn('roleList', body['data'])
        self.assertEqual(len(body['data']['roleList']), 1)

    def test_get_current_user(self):
        """获取当前用户信息"""
        res = self.client.get('/sys/users/current/')
        body = self.assertResponseOK(res)
        self.assertEqual(body['data']['username'], 'admin')


# ─────────────────────────────────────────────
# 个人中心
# ─────────────────────────────────────────────
class UserCenterTest(BaseTestCase):

    def test_update_user_info(self):
        """更新个人信息"""
        res = self.client.post('/sys/usercenter/updateUserInfo/', {
            'phonenumber': '13800138000',
            'email': 'center@test.com',
        }, format='json')
        self.assertResponseOK(res)
        self.user.refresh_from_db()
        self.assertEqual(self.user.email, 'center@test.com')

    def test_update_password_success(self):
        """修改密码成功"""
        res = self.client.post('/sys/usercenter/updateUserPassword/', {
            'old_password': 'admin123',
            'new_password': 'newpass456',
        }, format='json')
        self.assertResponseOK(res)
        self.user.refresh_from_db()
        self.assertTrue(check_password('newpass456', self.user.password))

    def test_login_plaintext_password_auto_migrates_to_hash(self):
        """历史明文密码用户登录成功后自动迁移为哈希"""
        legacy_user = SysUser.objects.create(username='legacy', password='legacy123', status=1)
        res = self.client.post('/sys/login', {
            'username': 'legacy', 'password': 'legacy123'
        }, format='json')
        body = res.json()
        self.assertEqual(body['code'], 200)
        legacy_user.refresh_from_db()
        self.assertNotEqual(legacy_user.password, 'legacy123')
        self.assertTrue(check_password('legacy123', legacy_user.password))

    def test_update_password_wrong_old(self):
        """旧密码错误修改失败"""
        res = self.client.post('/sys/usercenter/updateUserPassword/', {
            'old_password': 'wrongold',
            'new_password': 'newpass456',
        }, format='json')
        self.assertNotEqual(res.json()['code'], 200)


class ApiTokenTest(BaseTestCase):

    def test_create_agent_token_defaults_to_global(self):
        """不传 agent_id 时创建 agent 共享 token，agent_id 固定为 global"""
        res = self.client.post('/sys/usercenter/createApiToken/', {
            'bind_mode': 'agent',
            'name': '共享令牌',
        }, format='json')
        body = self.assertResponseOK(res)
        self.assertEqual(body['data']['agent_id'], 'global')
        self.assertEqual(body['data']['bind_mode'], 'agent')
        record = ApiToken.objects.filter(agent_id='global', bind_mode='agent').order_by('-id').first()
        if record is None:
            self.fail('expected a global agent token record')
        self.assertEqual(record.bind_mode, 'agent')

    def test_create_agent_token_can_be_multiple(self):
        """agent 共享 token 允许创建多个"""
        first = self.client.post('/sys/usercenter/createApiToken/', {
            'bind_mode': 'agent',
            'name': '共享令牌1',
        }, format='json')
        self.assertResponseOK(first)

        second = self.client.post('/sys/usercenter/createApiToken/', {
            'bind_mode': 'agent',
            'name': '共享令牌2',
        }, format='json')
        self.assertResponseOK(second)

        self.assertEqual(ApiToken.objects.filter(bind_mode='agent', agent_id='global').count(), 2)

    def test_create_api_token(self):
        """api 模式必须绑定具体 agent_id"""
        res = self.client.post('/sys/usercenter/createApiToken/', {
            'bind_mode': 'api',
            'agent_id': 'agent-001',
            'name': '独占令牌',
        }, format='json')
        body = self.assertResponseOK(res)
        self.assertEqual(body['data']['agent_id'], 'agent-001')
        self.assertEqual(body['data']['bind_mode'], 'api')
        record = ApiToken.objects.get(agent_id='agent-001')
        self.assertEqual(record.bind_mode, 'api')

    def test_create_api_token_expires_at_uses_user_timezone_and_store_utc(self):
        """api 模式无时区 expires_at 按用户时区解释并转为 UTC 存储"""
        # 当前测试用户时区为 Asia/Shanghai，12:00 本地时间应入库为 04:00 UTC。
        res = self.client.post('/sys/usercenter/createApiToken/', {
            'bind_mode': 'api',
            'agent_id': 'agent-tz-001',
            'name': '时区测试',
            'expires_at': '2030-01-01T12:00:00',
        }, format='json')
        self.assertResponseOK(res)

        record = ApiToken.objects.get(agent_id='agent-tz-001')
        self.assertIsNotNone(record.expires_at)
        expires_at = record.expires_at
        if expires_at is None:
            self.fail('expected expires_at to be set for api token')
        self.assertEqual(expires_at.tzinfo, dt_timezone.utc)
        self.assertEqual(expires_at.year, 2030)
        self.assertEqual(expires_at.month, 1)
        self.assertEqual(expires_at.day, 1)
        self.assertEqual(expires_at.hour, 4)
        self.assertEqual(expires_at.minute, 0)

    def test_create_agent_token_ignores_expires_at(self):
        """agent 模式即使传 expires_at 也应永不过期"""
        future_time = (timezone.now() + timedelta(days=30)).isoformat()
        res = self.client.post('/sys/usercenter/createApiToken/', {
            'bind_mode': 'agent',
            'name': '共享令牌',
            'expires_at': future_time,
        }, format='json')
        body = self.assertResponseOK(res)
        self.assertIsNone(body['data']['expires_at'])
        record = ApiToken.objects.filter(agent_id='global', bind_mode='agent').order_by('-id').first()
        if record is None:
            self.fail('expected a global agent token record')
        self.assertIsNone(record.expires_at)

    def test_create_token_invalid_bind_mode(self):
        """非法 bind_mode 应返回错误"""
        res = self.client.post('/sys/usercenter/createApiToken/', {
            'bind_mode': 'invalid_mode',
            'agent_id': 'agent-002',
        }, format='json')
        self.assertNotEqual(res.json()['code'], 200)

    def test_agent_tokens_list_contains_bind_mode_and_creator(self):
        """列表接口应返回 bind_mode 与创建人用户名"""
        ApiToken.objects.create(
            agent_id='agent-003',
            bind_mode='api',
            token_hash=make_password('x'),
            created_by=self.user,
        )
        res = self.client.get('/sys/usercenter/apiTokens/')
        body = self.assertResponseOK(res)
        self.assertGreaterEqual(body['data']['count'], 1)
        first = body['data']['results'][0]
        self.assertIn('bind_mode', first)
        self.assertIn('created_by_username', first)

    def test_rotate_disabled_token_should_fail(self):
        """已禁用 token 不允许轮换"""
        ApiToken.objects.create(
            agent_id='agent-004',
            bind_mode='api',
            token_hash=make_password('x'),
            is_active=False,
            created_by=self.user,
        )
        res = self.client.post('/sys/usercenter/rotateApiToken/', {
            'id': ApiToken.objects.get(agent_id='agent-004').id,
        }, format='json')
        self.assertNotEqual(res.json()['code'], 200)

    def test_disable_token_and_repeat_disable(self):
        """禁用接口首次成功，再次禁用返回错误"""
        ApiToken.objects.create(
            agent_id='agent-005',
            bind_mode='api',
            token_hash=make_password('x'),
            is_active=True,
            created_by=self.user,
        )
        token_id = ApiToken.objects.get(agent_id='agent-005').id
        first = self.client.post('/sys/usercenter/disableApiToken/', {
            'id': token_id,
        }, format='json')
        first_body = self.assertResponseOK(first)
        self.assertEqual(first_body['data']['is_active'], False)

        second = self.client.post('/sys/usercenter/disableApiToken/', {
            'id': token_id,
        }, format='json')
        self.assertNotEqual(second.json()['code'], 200)

    def test_delete_token_success_and_repeat_delete(self):
        """删除接口首次成功，再次删除返回不存在"""
        ApiToken.objects.create(
            agent_id='agent-006',
            bind_mode='api',
            token_hash=make_password('x'),
            is_active=True,
            created_by=self.user,
        )
        token_id = ApiToken.objects.get(agent_id='agent-006').id

        first = self.client.post('/sys/usercenter/deleteApiToken/', {
            'id': token_id,
        }, format='json')
        first_body = self.assertResponseOK(first)
        self.assertEqual(first_body['data']['deleted'], True)
        self.assertFalse(ApiToken.objects.filter(agent_id='agent-006').exists())

        second = self.client.post('/sys/usercenter/deleteApiToken/', {
            'id': token_id,
        }, format='json')
        self.assertNotEqual(second.json()['code'], 200)
