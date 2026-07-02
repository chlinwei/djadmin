from django.test import TestCase
from rest_framework.test import APIClient
from rest_framework_jwt.settings import api_settings

from menu.models import SysMenu
from user.models import SysUser


def _get_token(user: SysUser) -> str:
	jwt_payload_handler = api_settings.JWT_PAYLOAD_HANDLER  # type: ignore[operator]
	jwt_encode_handler = api_settings.JWT_ENCODE_HANDLER  # type: ignore[operator]
	payload = jwt_payload_handler(user)  # type: ignore[operator]
	return jwt_encode_handler(payload)  # type: ignore[operator]


class MenuManageTest(TestCase):
	def setUp(self):
		self.client = APIClient()
		self.user = SysUser.objects.create(username='admin', password='admin123', status=1)
		token = _get_token(self.user)
		self.client.credentials(HTTP_AUTHORIZATION=token)

	def assertResponseOK(self, res):
		body = res.json()
		self.assertIn('code', body, msg=f"响应缺少 'code' 字段: {body}")
		self.assertIn('msg', body, msg=f"响应缺少 'msg' 字段: {body}")
		self.assertIn('data', body, msg=f"响应缺少 'data' 字段: {body}")
		self.assertEqual(body['code'], 200, msg=f"Expected code=200, got: {body}")
		return body

	def test_create_menu(self):
		res = self.client.post(
			'/sys/menus/',
			{
				'name': '测试操作审计目录',
				'icon': 'shield-halved',
				'parent_id': 0,
				'order_num': 200,
				'path': '/audit-test',
				'component': '',
				'menu_type': 'M',
				'perms': '',
				'location': 1,
				'remark': '测试菜单',
			},
			format='json',
		)
		body = self.assertResponseOK(res)
		self.assertEqual(body['data']['name'], '测试操作审计目录')
		self.assertEqual(body['data']['path'], '/audit-test')

	def test_patch_menu(self):
		menu = SysMenu.objects.create(
			name='测试操作审计目录',
			icon='shield-halved',
			parent_id=0,
			order_num=200,
			path='/audit-test',
			component='',
			menu_type='M',
			perms='',
			location=1,
			remark='测试菜单',
		)

		res = self.client.patch(
			f'/sys/menus/{menu.id}/',
			{
				'name': '测试操作审计目录-已更新',
				'path': '/audit-test-updated',
			},
			format='json',
		)

		body = self.assertResponseOK(res)
		self.assertEqual(body['data']['id'], menu.id)
		self.assertEqual(body['data']['name'], '测试操作审计目录-已更新')
		self.assertEqual(body['data']['path'], '/audit-test-updated')

	def test_retrieve_menu(self):
		menu = SysMenu.objects.create(
			name='测试菜单详情',
			icon='bars',
			parent_id=0,
			order_num=201,
			path='/menu-detail',
			component='',
			menu_type='M',
			perms='',
			location=1,
			remark='测试菜单详情',
		)
		res = self.client.get(f'/sys/menus/{menu.id}/')
		body = self.assertResponseOK(res)
		self.assertEqual(body['data']['id'], menu.id)
		self.assertEqual(body['data']['name'], '测试菜单详情')
