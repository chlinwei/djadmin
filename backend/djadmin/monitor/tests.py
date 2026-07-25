from django.test import TestCase
from rest_framework.test import APIClient
from rest_framework_jwt.settings import api_settings

from assets.models import Host
from sys_config.models import SysConfig
from user.models import SysUser

from .models import MonitorTarget


class MonitorSmokeTest(TestCase):
    def test_monitor_module_importable(self):
        self.assertTrue(True)


def _get_token(user: SysUser) -> str:
    jwt_payload_handler = api_settings.JWT_PAYLOAD_HANDLER  # type: ignore[operator]
    jwt_encode_handler = api_settings.JWT_ENCODE_HANDLER  # type: ignore[operator]
    payload = jwt_payload_handler(user)  # type: ignore[operator]
    return jwt_encode_handler(payload)  # type: ignore[operator]


class MonitorTargetDeleteTest(TestCase):
    """MonitorViewSet.destroy() 的前置校验：必须先关闭纳管（managed_enabled=False）
    且没有仍在进行中的卸载任务（install_status != pending）才允许删除记录。"""

    def setUp(self):
        self.client = APIClient()
        self.user = SysUser.objects.create(username='admin', password='admin123', status=1)
        token = _get_token(self.user)
        self.client.credentials(HTTP_AUTHORIZATION=token)
        self.host = Host.objects.create(instance_name='monitor-del-host', ip='192.168.1.210', port=22)

    def assertResponseOK(self, res):
        body = res.json()
        self.assertIn('code', body)
        self.assertIn('msg', body)
        self.assertIn('data', body)
        self.assertEqual(body['code'], 200, msg=f"Expected code=200, got: {body}")
        return body

    def test_delete_rejected_when_managed_enabled(self):
        """仍处于纳管开启状态时，删除应被拒绝，记录不能被删掉。"""
        target = MonitorTarget.objects.create(
            host=self.host, exporter_type='node_exporter', managed_enabled=True,
            install_status=MonitorTarget.InstallStatus.SUCCESS,
        )
        res = self.client.delete(f'/sys/monitor/targets/{target.id}/')
        body = res.json()
        self.assertEqual(body['code'], 400)
        self.assertTrue(MonitorTarget.objects.filter(id=target.id).exists())

    def test_delete_rejected_when_uninstall_pending(self):
        """已关闭纳管但卸载任务还没跑完（install_status=pending）时，删除应被拒绝。"""
        target = MonitorTarget.objects.create(
            host=self.host, exporter_type='node_exporter', managed_enabled=False,
            install_status=MonitorTarget.InstallStatus.PENDING,
        )
        res = self.client.delete(f'/sys/monitor/targets/{target.id}/')
        body = res.json()
        self.assertEqual(body['code'], 400)
        self.assertTrue(MonitorTarget.objects.filter(id=target.id).exists())

    def test_delete_succeeds_when_disabled_and_not_pending(self):
        """已关闭纳管且卸载任务已经有终态（如 success）时，允许删除记录。"""
        target = MonitorTarget.objects.create(
            host=self.host, exporter_type='node_exporter', managed_enabled=False,
            install_status=MonitorTarget.InstallStatus.SUCCESS,
        )
        res = self.client.delete(f'/sys/monitor/targets/{target.id}/')
        self.assertResponseOK(res)
        self.assertFalse(MonitorTarget.objects.filter(id=target.id).exists())


class PrometheusHttpSDTest(TestCase):
    def setUp(self):
        self.client = APIClient()
        SysConfig.objects.update_or_create(
            key='monitor.prometheus.http_sd_token',
            defaults={
                'name': 'Prometheus HTTP SD Token',
                'value': 'test-token',
                'default_value': 'test-token',
                'value_type': 'string',
                'is_readonly': False,
                'description': 'test',
            },
        )

    def test_http_sd_requires_valid_token(self):
        res = self.client.get('/sys/monitor/targets/prometheus/http-sd/?token=wrong-token')
        self.assertEqual(res.status_code, 403)

    def test_http_sd_returns_targets_with_port_resolution(self):
        host1 = Host.objects.create(instance_name='node-host-1', ip='10.0.0.11', port=22)
        host2 = Host.objects.create(instance_name='cad-host-1', ip='10.0.0.12', port=22)
        host3 = Host.objects.create(instance_name='custom-host-1', ip='10.0.0.13', port=22)

        MonitorTarget.objects.create(
            host=host1,
            exporter_type='node_exporter',
            scrape_port=9100,
            managed_enabled=True,
            install_status=MonitorTarget.InstallStatus.SUCCESS,
            labels={},
        )
        MonitorTarget.objects.create(
            host=host2,
            exporter_type='cadvisor',
            scrape_port=18080,
            managed_enabled=True,
            install_status=MonitorTarget.InstallStatus.SUCCESS,
            labels={'scrape_port': 18080},
        )
        MonitorTarget.objects.create(
            host=host3,
            exporter_type='custom_exporter',
            scrape_port=19090,
            managed_enabled=True,
            install_status=MonitorTarget.InstallStatus.SUCCESS,
            labels={},
        )

        res = self.client.get('/sys/monitor/targets/prometheus/http-sd/?token=test-token')
        self.assertEqual(res.status_code, 200)
        payload = res.json()
        self.assertIsInstance(payload, list)

        by_target = {item['targets'][0]: item for item in payload}
        self.assertIn('10.0.0.11:9100', by_target)
        self.assertIn('10.0.0.12:18080', by_target)
        self.assertIn('10.0.0.13:19090', by_target)

        labels = by_target['10.0.0.12:18080']['labels']
        self.assertEqual(labels['__meta_dj_exporter_type'], 'cadvisor')
        self.assertEqual(labels['__meta_dj_instance_name'], 'cad-host-1')

