import tempfile
from email.message import Message
from io import BytesIO
from urllib.error import HTTPError

from django.test import TestCase
from django.test.utils import override_settings
from django.utils import timezone
from rest_framework.test import APIClient
from rest_framework_jwt.settings import api_settings

from django.contrib.auth.hashers import make_password

from assets.models import Host
from user.models import ApiToken, SysUser

from .models import MonitorTarget


class MonitorSmokeTest(TestCase):
    def test_monitor_module_importable(self):
        self.assertTrue(True)

    def test_managed_target_serializers_expose_explicit_types(self):
        from .models import LogCollectionTarget
        from .serializer import LogCollectionTargetSerializer, MonitorTargetSerializer

        host = Host.objects.create(instance_name='typed-target-host', ip='10.8.0.1')
        exporter_target = MonitorTarget.objects.create(host=host, exporter_type='node_exporter')
        fluent_bit_target = LogCollectionTarget.objects.create(host=host)

        self.assertEqual(MonitorTargetSerializer(exporter_target).data['target_type'], 'exporter')
        self.assertEqual(LogCollectionTargetSerializer(fluent_bit_target).data['target_type'], 'fluent_bit')

    def test_create_fluent_bit_managed_target_and_reject_duplicate_host(self):
        client = APIClient()
        user = SysUser.objects.create(username='admin', password='admin123', status=1)
        client.credentials(HTTP_AUTHORIZATION=_get_token(user))
        host = Host.objects.create(instance_name='fluent-target-host', ip='10.8.0.2')

        response = client.post('/monitor/log-targets/', {'host': host.id}, format='json')
        self.assertEqual(response.json()['code'], 200)
        self.assertEqual(response.json()['data']['target_type'], 'fluent_bit')

        duplicate = client.post('/monitor/log-targets/', {'host': host.id}, format='json')
        self.assertEqual(duplicate.json()['code'], 400)


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
        self.host = Host.objects.create(instance_name='monitor-del-host', ip='192.168.1.210')

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
        res = self.client.delete(f'/monitor/targets/{target.id}/')
        body = res.json()
        self.assertEqual(body['code'], 400)
        self.assertTrue(MonitorTarget.objects.filter(id=target.id).exists())

    def test_delete_rejected_when_uninstall_pending(self):
        """已关闭纳管但卸载任务还没跑完（install_status=pending）时，删除应被拒绝。"""
        target = MonitorTarget.objects.create(
            host=self.host, exporter_type='node_exporter', managed_enabled=False,
            install_status=MonitorTarget.InstallStatus.PENDING,
        )
        res = self.client.delete(f'/monitor/targets/{target.id}/')
        body = res.json()
        self.assertEqual(body['code'], 400)
        self.assertTrue(MonitorTarget.objects.filter(id=target.id).exists())

    def test_delete_succeeds_when_disabled_and_not_pending(self):
        """已关闭纳管且卸载任务已经有终态（如 success）时，允许删除记录。"""
        target = MonitorTarget.objects.create(
            host=self.host, exporter_type='node_exporter', managed_enabled=False,
            install_status=MonitorTarget.InstallStatus.SUCCESS,
        )
        res = self.client.delete(f'/monitor/targets/{target.id}/')
        self.assertResponseOK(res)
        self.assertFalse(MonitorTarget.objects.filter(id=target.id).exists())


class PrometheusHttpSDTest(TestCase):
    def setUp(self):
        self.client = APIClient()
        # http_sd 与 dj-agent 共用全局 ApiToken 认证：用 agent 共享 token 校验（?token=）。
        ApiToken.objects.create(
            agent_id='prometheus-http-sd',
            bind_mode='agent',
            token_hash=make_password('test-token'),
            is_active=True,
        )

    def test_http_sd_requires_valid_token(self):
        res = self.client.get('/monitor/targets/prometheus/http-sd/?token=wrong-token')
        self.assertEqual(res.status_code, 403)

    def test_http_sd_returns_targets_with_port_resolution(self):
        host1 = Host.objects.create(instance_name='node-host-1', ip='10.0.0.11')
        host2 = Host.objects.create(instance_name='cad-host-1', ip='10.0.0.12')
        host3 = Host.objects.create(instance_name='custom-host-1', ip='10.0.0.13')

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

        res = self.client.get('/monitor/targets/prometheus/http-sd/?token=test-token')
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


from unittest import mock
from types import SimpleNamespace

from assets.models import (
    Application,
    ApplicationDeployment,
    ApplicationDeploymentTemplate,
    ApplicationLogDefinition,
    ApplicationService,
    ApplicationServiceDeployment,
    ApplicationVersion,
    BusinessEnvironment,
    BusinessSystem,
)

from .fluent_bit import (
    build_host_fragments,
    config_fingerprint,
    render_input_fragment,
    render_output_fragment,
)
from .log_management import (
    RETENTION_TIERS,
    build_default_pipeline_body,
    build_index_name,
    build_index_template_body,
    build_ism_policy_body,
    build_pipeline_name,
)
from .models import LogCollectionTarget, LogProcessingRule, OpenSearchCluster
from .opensearch_client import OpenSearchClient, OpenSearchError


class OpenSearchClientTest(TestCase):
    def setUp(self):
        self.cluster = OpenSearchCluster.objects.create(
            name='client-test', hosts='https://opensearch.example:9200', index_prefix='logs',
        )

    @mock.patch('monitor.opensearch_client.urllib_request.urlopen')
    def test_list_pipelines_treats_404_empty_object_as_empty_collection(self, urlopen):
        urlopen.side_effect = HTTPError(
            url='https://opensearch.example:9200/_ingest/pipeline',
            code=404,
            msg='Not Found',
            hdrs=Message(),
            fp=BytesIO(b'{}'),
        )

        self.assertEqual(OpenSearchClient(self.cluster).list_pipelines(), {})

    @mock.patch('monitor.opensearch_client.urllib_request.urlopen')
    def test_named_pipeline_keeps_404_as_error(self, urlopen):
        urlopen.side_effect = HTTPError(
            url='https://opensearch.example:9200/_ingest/pipeline/missing',
            code=404,
            msg='Not Found',
            hdrs=Message(),
            fp=BytesIO(b'{}'),
        )

        with self.assertRaises(OpenSearchError):
            OpenSearchClient(self.cluster).get_pipeline('missing')


class LogManagementBuilderTest(TestCase):
    """log_management 纯构建器：索引命名、template、ISM、默认 pipeline（架构文档 §4/§5）。"""

    def test_build_index_name(self):
        self.assertEqual(build_index_name('logs', 'prod', 'tib', 'hot'), 'logs-prod-tib-hot')
        # 非法字符统一归一为小写安全段，避免生成非法索引名
        self.assertEqual(build_index_name('Logs', 'Test Env', 'ESB', 'std'), 'logs-test-env-esb-std')

    def test_build_index_name_rejects_unknown_tier(self):
        with self.assertRaises(ValueError):
            build_index_name('logs', 'prod', 'tib', 'warm')

    def test_build_index_template_body(self):
        body = build_index_template_body('logs')
        self.assertEqual(body['index_patterns'], ['logs-*'])
        self.assertEqual(body['template']['settings']['index.mapping.total_fields.limit'], 2000)
        self.assertIn('data_stream', body)

    def test_build_ism_policy_body_all_tiers(self):
        for tier, config in RETENTION_TIERS.items():
            body = build_ism_policy_body('logs', tier)
            policy = body['policy']
            # ism_template 按索引名后缀自动挂载，新建业务无需手工配置
            self.assertEqual(policy['ism_template'][0]['index_patterns'], [f'logs-*-{tier}'])
            hot_state = policy['states'][0]
            self.assertEqual(
                hot_state['actions'][0]['rollover']['min_index_age'],
                config['rollover_min_index_age'],
            )
            self.assertEqual(
                hot_state['transitions'][0]['conditions']['min_index_age'],
                f"{config['retention_days']}d",
            )

    def test_build_default_pipeline_body(self):
        body = build_default_pipeline_body()
        processor_types = [next(iter(item)) for item in body['processors']]
        # 必须包含 date（覆盖 @timestamp）与 fingerprint（错误聚合基础）
        self.assertIn('date', processor_types)
        self.assertIn('fingerprint', processor_types)
        # 必须配置 on_failure，否则单条格式不符会丢整条日志
        self.assertTrue(body.get('on_failure'))

    def test_build_pipeline_name(self):
        self.assertEqual(build_pipeline_name('tomcat', 'catalina'), 'app-tomcat-catalina')


class FluentBitRenderTest(TestCase):
    """fluent_bit 片段渲染与指纹（架构文档 §8）。"""

    RECORDS = {'business_system': 'tib', 'environment': 'test', 'service': 'tomcat'}

    def test_render_input_fragment_with_multiline(self):
        content = render_input_fragment(
            application_code='tomcat',
            service_code='tomcat-svc',
            instance_name='kul-tib-tomcat1',
            log_name='catalina',
            log_path='/home/esb/tomcat/logs/catalina.out',
            tag='tomcat.tomcat-svc.kul-tib-tomcat1.catalina',
            multiline_rule=SimpleNamespace(
                name='timestamp-lines',
                start_pattern=r'^\d{4}-\d{2}-\d{2}',
                continuation_pattern=r'^(?!\d{4}-\d{2}-\d{2})',
                flush_timeout=1000,
            ),
            encoding='utf-8',
            records=self.RECORDS,
        )
        self.assertIn('[MULTILINE_PARSER]', content)
        self.assertIn('Name          multiline_tomcat.tomcat-svc.kul-tib-tomcat1.catalina', content)
        self.assertIn('Rule          "start_state" "/^\\d{4}-\\d{2}-\\d{2}/" "continuation"', content)
        self.assertIn('Multiline.parser  multiline_tomcat.tomcat-svc.kul-tib-tomcat1.catalina', content)
        self.assertIn('DB                /var/lib/fluent-bit/tomcat__tomcat-svc__kul-tib-tomcat1__catalina.db', content)
        self.assertIn('Record  business_system tib', content)
        self.assertIn('Match   tomcat.tomcat-svc.kul-tib-tomcat1.catalina', content)

    def test_render_input_fragment_without_multiline(self):
        content = render_input_fragment(
            application_code='nginx',
            service_code='nginx-svc',
            instance_name='nginx1',
            log_name='access',
            log_path='/var/log/nginx/access.log',
            tag='nginx.nginx1',
            records=self.RECORDS,
        )
        self.assertNotIn('Multiline.parser', content)

    def test_render_output_fragment(self):
        content = render_output_fragment(
            application_code='tomcat', service_code='tomcat-svc', log_name='catalina',
            index='logs-test-tib-std', pipeline='app-tomcat-catalina',
        )
        self.assertIn('Name                opensearch', content)
        self.assertIn('Match               tomcat.tomcat-svc.*.catalina', content)
        self.assertIn('Index               logs-test-tib-std', content)
        self.assertIn('Pipeline            app-tomcat-catalina', content)
        # 凭据不写入配置文件，经 systemd Environment 注入
        self.assertIn('${OS_PASSWORD}', content)

    def test_config_fingerprint_stable_and_order_insensitive(self):
        fragments = ['a.conf-content', 'b.conf-content']
        self.assertEqual(config_fingerprint(fragments), config_fingerprint(list(reversed(fragments))))
        self.assertNotEqual(config_fingerprint(fragments), config_fingerprint(['changed']))


class LogCollectionApiTest(TestCase):
    """日志采集 API：pipeline 调试、render-config 预览，均校验统一响应格式。"""

    def setUp(self):
        self.client = APIClient()
        self.user = SysUser.objects.create(username='admin', password='admin123', status=1)
        self.client.credentials(HTTP_AUTHORIZATION=_get_token(self.user))
        self.cluster = OpenSearchCluster.objects.create(
            name='test', hosts='https://10.25.66.150:9200', index_prefix='logs',
        )

    def assertResponseOK(self, res):
        body = res.json()
        self.assertIn('code', body)
        self.assertIn('msg', body)
        self.assertIn('data', body)
        self.assertEqual(body['code'], 200, msg=f"Expected code=200, got: {body}")
        return body

    def _build_service_graph(self, host):
        application = Application.objects.create(name='Tomcat', code='tomcat')
        system = BusinessSystem.objects.create(name='TIB', code='tib')
        environment = BusinessEnvironment.objects.create(name='测试', code='test')
        version = ApplicationVersion.objects.create(application=application, version='9.0.35')
        template = ApplicationDeploymentTemplate.objects.create(
            application=application, name='默认模板', control_type='command',
            run_user='esb', app_home='/home/esb/tomcat',
        )
        rule = LogProcessingRule.objects.create(
            cluster=self.cluster, name='app-tomcat-catalina', multiline_enabled=True,
            start_pattern=r'^\d{4}-\d{2}-\d{2}', continuation_pattern=r'^(?!\d{4}-\d{2}-\d{2})',
            pipeline_body={'processors': []},
        )
        log_def = ApplicationLogDefinition.objects.create(
            deployment_template=template, name='catalina',
            path_pattern='${APP_HOME}/logs/catalina.out',
            collection_enabled=True, processing_rule=rule,
        )
        deployment = ApplicationDeployment.objects.create(host=host, instance_name='kul-tib-tomcat1')
        service = ApplicationService.objects.create(
            business_system=system, environment=environment, application=application,
            application_version=version, deployment_template=template,
            name='tomcat服务', code='tomcat-svc', log_collection_enabled=True,
        )
        ApplicationServiceDeployment.objects.create(service=service, deployment=deployment)
        return service, log_def

    def test_bootstrap_calls_storage_bootstrap(self):
        with mock.patch('monitor.views.bootstrap_log_storage', return_value={
            'index_template': 'logs-template',
            'ism_policies': ['logs-hot-retention', 'logs-std-retention', 'logs-cold-retention'],
        }) as mocked:
            res = self.client.post(f'/monitor/opensearch-clusters/{self.cluster.id}/bootstrap/')
        body = self.assertResponseOK(res)
        mocked.assert_called_once()
        self.assertEqual(body['data']['index_template'], 'logs-template')
        self.assertEqual(len(body['data']['ism_policies']), 3)

    def test_simulate_requires_docs(self):
        res = self.client.post(
            f'/monitor/opensearch-clusters/{self.cluster.id}/pipeline-simulate/',
            {'pipeline': {'processors': []}}, format='json',
        )
        self.assertEqual(res.json()['code'], 400)

    def test_simulate_with_inline_pipeline(self):
        with mock.patch('monitor.views.OpenSearchClient') as client_cls:
            client_cls.return_value.simulate_pipeline_body.return_value = {'docs': [{'doc': {'_source': {'ok': True}}}]}
            res = self.client.post(
                f'/monitor/opensearch-clusters/{self.cluster.id}/pipeline-simulate/',
                {'pipeline': {'processors': []}, 'docs': [{'message': 'x'}]}, format='json',
            )
        body = self.assertResponseOK(res)
        self.assertIn('docs', body['data'])

    def test_create_processing_rule_publishes_same_pipeline(self):
        payload = {
            'cluster': self.cluster.id,
            'name': 'timestamp-business-log',
            'description': '时间开头的通用多行日志',
            'input_format': 'text',
            'multiline_enabled': True,
            'start_pattern': r'^\d{4}-\d{2}-\d{2}',
            'continuation_pattern': r'^(?!\d{4}-\d{2}-\d{2})',
            'flush_timeout': 1000,
            'pipeline_body': {'processors': []},
        }
        with mock.patch('monitor.views.OpenSearchClient') as client_cls:
            client_cls.return_value.put_pipeline.return_value = {'acknowledged': True}
            res = self.client.post('/monitor/log-processing-rules/', payload, format='json')

        body = self.assertResponseOK(res)
        self.assertEqual(body['data']['name'], payload['name'])
        client_cls.return_value.put_pipeline.assert_called_once_with(
            payload['name'], payload['pipeline_body'],
        )

    def test_processing_rule_requires_patterns_when_multiline_enabled(self):
        res = self.client.post('/monitor/log-processing-rules/', {
            'cluster': self.cluster.id,
            'name': 'invalid-multiline',
            'input_format': 'text',
            'multiline_enabled': True,
            'pipeline_body': {'processors': []},
        }, format='json')

        self.assertNotEqual(res.json()['code'], 200)
        self.assertFalse(LogProcessingRule.objects.filter(name='invalid-multiline').exists())

    def test_processing_rule_cannot_be_deleted_while_referenced(self):
        host = Host.objects.create(instance_name='rule-host', ip='10.0.1.20')
        _, log_definition = self._build_service_graph(host)

        with mock.patch('monitor.views.OpenSearchClient') as client_cls:
            res = self.client.delete(
                f'/monitor/log-processing-rules/{log_definition.processing_rule_id}/',
            )

        self.assertEqual(res.json()['code'], 400)
        client_cls.assert_not_called()

    def test_render_config_requires_cluster(self):
        OpenSearchCluster.objects.all().delete()
        host = Host.objects.create(instance_name='log-host-1', ip='10.0.1.11')
        target = LogCollectionTarget.objects.create(host=host)
        res = self.client.get(f'/monitor/log-targets/{target.id}/render-config/')
        self.assertEqual(res.json()['code'], 400)

    def test_render_config_generates_fragments(self):
        host = Host.objects.create(instance_name='log-host-2', ip='10.0.1.12')
        self._build_service_graph(host)
        target = LogCollectionTarget.objects.create(host=host)

        res = self.client.get(f'/monitor/log-targets/{target.id}/render-config/')
        body = self.assertResponseOK(res)
        inputs = body['data']['inputs']
        # ${APP_HOME} 已展开为绝对路径，Tag 按应用、服务、实例、日志四段隔离。
        fragment = inputs['tomcat__tomcat-svc__kul-tib-tomcat1__catalina.conf']
        self.assertIn('Path              /home/esb/tomcat/logs/catalina.out', fragment)
        self.assertIn('Tag               tomcat.tomcat-svc.kul-tib-tomcat1.catalina', fragment)
        self.assertIn('Multiline.parser  multiline_tomcat.tomcat-svc.kul-tib-tomcat1.catalina', fragment)
        # 索引名带保留档位后缀，ISM 据此挂载保留策略
        self.assertIn('logs-test-tib-std', body['data']['outputs']['tomcat__tomcat-svc__catalina.conf'])
        # 指纹未下发过，不应判定为已同步
        self.assertFalse(body['data']['up_to_date'])

    def test_render_config_keeps_multiple_logs_for_same_instance(self):
        host = Host.objects.create(instance_name='log-host-multiple', ip='10.0.1.15')
        _, first_log = self._build_service_graph(host)
        ApplicationLogDefinition.objects.create(
            deployment_template=first_log.deployment_template,
            name='access',
            path_pattern='${APP_HOME}/logs/access.log',
            collection_enabled=True,
            processing_rule=first_log.processing_rule,
        )
        target = LogCollectionTarget.objects.create(host=host)

        res = self.client.get(f'/monitor/log-targets/{target.id}/render-config/')
        body = self.assertResponseOK(res)

        self.assertEqual(len(body['data']['inputs']), 2)
        self.assertEqual(len(body['data']['outputs']), 2)
        self.assertIn('tomcat__tomcat-svc__kul-tib-tomcat1__catalina.conf', body['data']['inputs'])
        self.assertIn('tomcat__tomcat-svc__kul-tib-tomcat1__access.conf', body['data']['inputs'])

    def test_render_config_rejects_duplicate_paths(self):
        """同主机多实例展开出相同日志路径时必须拦截，避免 harvester 冲突（架构文档 §12）。"""
        host = Host.objects.create(instance_name='log-host-3', ip='10.0.1.13')
        service, _ = self._build_service_graph(host)
        duplicate = ApplicationDeployment.objects.create(host=host, instance_name='kul-tib-tomcat2')
        ApplicationServiceDeployment.objects.create(service=service, deployment=duplicate)
        target = LogCollectionTarget.objects.create(host=host)

        res = self.client.get(f'/monitor/log-targets/{target.id}/render-config/')
        self.assertEqual(res.json()['code'], 400)
        self.assertIn('路径冲突', res.json()['msg'])

    def test_host_fragments_skip_disabled_collection(self):
        """服务开关 OFF 时不生成任何片段（架构文档 §6 两层开关）。"""
        host = Host.objects.create(instance_name='log-host-4', ip='10.0.1.14')
        service, _ = self._build_service_graph(host)
        service.log_collection_enabled = False
        service.save(update_fields=['log_collection_enabled'])

        fragments = build_host_fragments(host, self.cluster)
        self.assertEqual(fragments['inputs'], {})
        self.assertEqual(fragments['outputs'], {})


class SoftwarePackageGenericTest(TestCase):
    """软件仓库通用化：软件包记录决定元数据与存储目录，文件名不参与解析。"""

    def setUp(self):
        self._media_dir = tempfile.TemporaryDirectory()
        self._media_override = override_settings(MEDIA_ROOT=self._media_dir.name)
        self._media_override.enable()
        self.addCleanup(self._media_dir.cleanup)
        self.addCleanup(self._media_override.disable)
        # --keepdb 重跑时上次用例创建的记录仍在库中，先按 name 清掉，保证用例幂等
        from .models import SoftwarePackage
        SoftwarePackage.objects.filter(name__in=['fluent-bit', 'fluent_bit']).delete()
        self.client = APIClient()
        self.user = SysUser.objects.create(username='admin', password='admin123', status=1)
        self.client.credentials(HTTP_AUTHORIZATION=_get_token(self.user))

    def assertResponseOK(self, res):
        body = res.json()
        self.assertEqual(body['code'], 200, msg=f"Expected code=200, got: {body}")
        return body

    def test_create_placeholder_package_without_file(self):
        """先建“未同步”占位记录（不带文件），后续行内上传补全。"""
        res = self.client.post('/monitor/packages/', {
            'package_type': 'fluent_bit',
            'name': 'fluent-bit', 'version': '0.0.0', 'os': 'linux', 'arch': 'amd64',
            'default_port': 2020, 'service_run_as_user': 'dj-agent',
        }, format='json')
        body = self.assertResponseOK(res)
        self.assertEqual(body['data']['name'], 'fluent-bit')
        self.assertEqual(body['data']['package_type'], 'fluent_bit')
        self.assertFalse(body['data']['synced'])

    def test_platform_specific_package_storage_names_do_not_collide(self):
        from .models import SoftwarePackage, software_package_upload_to

        common = {
            'name': 'fluent-bit', 'version': '5.1.1', 'os': 'linux', 'arch': 'amd64',
            'package_type': SoftwarePackage.PackageType.FLUENT_BIT,
            'platform_family': SoftwarePackage.PlatformFamily.RHEL,
            'package_format': SoftwarePackage.PackageFormat.RPM,
        }
        el7 = SoftwarePackage(platform_major='7', **common)
        el8 = SoftwarePackage(platform_major='8', **common)
        self.assertNotEqual(
            software_package_upload_to(el7, 'fluent-bit-5.1.1-1.x86_64.rpm'),
            software_package_upload_to(el8, 'fluent-bit-5.1.1-1.x86_64.rpm'),
        )
        self.assertEqual(
            software_package_upload_to(el7, 'fluent-bit-5.1.1.rhel7.x86_64.rpm'),
            'monitor_packages/fluentBit/amd64/rhel7/fluent-bit-5.1.1.rhel7.x86_64.rpm',
        )
        self.assertEqual(
            software_package_upload_to(el8, 'fluent-bit-3.2.10.rhel8.x86_64.rpm'),
            'monitor_packages/fluentBit/amd64/rhel8/fluent-bit-3.2.10.rhel8.x86_64.rpm',
        )

    def test_create_rejects_duplicate_name_version_arch(self):
        from .models import SoftwarePackage
        res1 = self.client.post('/monitor/packages/', {
            'package_type': 'fluent_bit',
            'name': 'fluent-bit', 'version': '3.1.9', 'os': 'linux', 'arch': 'amd64',
            'default_port': 2020, 'service_run_as_user': 'dj-agent',
        }, format='json')
        self.assertResponseOK(res1)
        res = self.client.post('/monitor/packages/', {
            'package_type': 'fluent_bit',
            'name': 'fluent-bit', 'version': '3.1.9', 'os': 'linux', 'arch': 'amd64',
            'default_port': 2020, 'service_run_as_user': 'dj-agent',
        }, format='json')
        # 唯一约束冲突必须转为统一错误响应，不能抛 500
        self.assertEqual(res.json()['code'], 400)

    def test_upload_accepts_non_node_exporter_filename(self):
        """行内上传保留记录版本，不从文件名提取任何元数据。"""
        from django.core.files.uploadedfile import SimpleUploadedFile

        from .models import SoftwarePackage
        pkg = SoftwarePackage.objects.create(
            package_type=SoftwarePackage.PackageType.FLUENT_BIT,
            name='fluent-bit', version='0.0.0', os='linux', arch='amd64',
            service_run_as_user='dj-agent',
        )
        upload = SimpleUploadedFile(
            'fluent-bit-3.1.9.linux-amd64.tar.gz', b'fake-tarball-content', content_type='application/gzip',
        )
        res = self.client.post(f'/monitor/packages/{pkg.id}/upload/', {'file': upload}, format='multipart')
        body = self.assertResponseOK(res)
        self.assertEqual(body['data']['version'], '0.0.0')
        self.assertIn('/fluentBit/amd64/linux/', body['data']['file'])
        self.assertTrue(body['data']['synced'])
        pkg.refresh_from_db()
        self.assertTrue(pkg.sha256)

    def test_upload_does_not_parse_name_prefix(self):
        """文件名前缀不参与校验，包类型和平台只取当前软件包记录。"""
        from django.core.files.uploadedfile import SimpleUploadedFile

        from .models import SoftwarePackage
        pkg = SoftwarePackage.objects.create(
            package_type=SoftwarePackage.PackageType.FLUENT_BIT,
            name='fluent-bit', version='0.0.0', os='linux', arch='amd64',
            service_run_as_user='dj-agent',
        )
        upload = SimpleUploadedFile(
            'node_exporter-1.8.2.linux-amd64.tar.gz', b'x', content_type='application/gzip',
        )
        res = self.client.post(f'/monitor/packages/{pkg.id}/upload/', {'file': upload}, format='multipart')
        body = self.assertResponseOK(res)
        self.assertIn('/fluentBit/amd64/linux/', body['data']['file'])

    def test_upload_accepts_fluent_bit_rpm(self):
        from django.core.files.uploadedfile import SimpleUploadedFile

        from .models import SoftwarePackage
        pkg = SoftwarePackage.objects.create(
            package_type=SoftwarePackage.PackageType.FLUENT_BIT,
            name='fluent-bit', version='3.2.10', os='linux', arch='amd64',
            package_format=SoftwarePackage.PackageFormat.RPM,
            platform_family=SoftwarePackage.PlatformFamily.RHEL,
            platform_major='9', service_run_as_user='dj-agent',
        )
        upload = SimpleUploadedFile(
            'fluent-bit-3.2.10-1.x86_64.rpm', b'fake-rpm', content_type='application/x-rpm',
        )
        body = self.assertResponseOK(
            self.client.post(f'/monitor/packages/{pkg.id}/upload/', {'file': upload}, format='multipart')
        )
        self.assertEqual(body['data']['version'], '3.2.10')
        self.assertTrue(body['data']['file'].endswith(
            '/fluentBit/amd64/rhel9/fluent-bit-3.2.10-1.x86_64.rpm'
        ))

    def test_upload_allows_same_rpm_version_for_different_rhel_majors(self):
        from django.core.files.uploadedfile import SimpleUploadedFile

        from .models import SoftwarePackage
        packages = [
            SoftwarePackage.objects.create(
                package_type=SoftwarePackage.PackageType.FLUENT_BIT,
                name='fluent-bit', version='3.2.10', os='linux', arch='amd64',
                package_format=SoftwarePackage.PackageFormat.RPM,
                platform_family=SoftwarePackage.PlatformFamily.RHEL,
                platform_major=major, service_run_as_user='dj-agent',
            )
            for major in ('8', '9')
        ]
        for package in packages:
            upload = SimpleUploadedFile(
                'fluent-bit-3.2.10-1.x86_64.rpm', b'fake-rpm', content_type='application/x-rpm',
            )
            body = self.assertResponseOK(
                self.client.post(
                    f'/monitor/packages/{package.id}/upload/', {'file': upload}, format='multipart',
                )
            )
            self.assertEqual(body['data']['version'], '3.2.10')
            self.assertIn(f'/fluentBit/amd64/rhel{package.platform_major}/', body['data']['file'])

    def test_upload_accepts_fluent_bit_deb(self):
        from django.core.files.uploadedfile import SimpleUploadedFile

        from .models import SoftwarePackage
        pkg = SoftwarePackage.objects.create(
            package_type=SoftwarePackage.PackageType.FLUENT_BIT,
            name='fluent-bit', version='3.2.10', os='linux', arch='arm64',
            package_format=SoftwarePackage.PackageFormat.DEB,
            platform_family=SoftwarePackage.PlatformFamily.UBUNTU,
            platform_major='22', service_run_as_user='dj-agent',
        )
        upload = SimpleUploadedFile(
            'fluent-bit_3.2.10_arm64.deb', b'fake-deb', content_type='application/vnd.debian.binary-package',
        )
        body = self.assertResponseOK(
            self.client.post(f'/monitor/packages/{pkg.id}/upload/', {'file': upload}, format='multipart')
        )
        self.assertEqual(body['data']['version'], '3.2.10')
        self.assertTrue(body['data']['file'].endswith(
            '/fluentBit/arm64/ubuntu22/fluent-bit_3.2.10_arm64.deb'
        ))

    def test_upload_rejects_file_extension_mismatch(self):
        from django.core.files.uploadedfile import SimpleUploadedFile

        from .models import SoftwarePackage
        pkg = SoftwarePackage.objects.create(
            package_type=SoftwarePackage.PackageType.FLUENT_BIT,
            name='fluent-bit', version='3.2.10', os='linux', arch='amd64',
            package_format=SoftwarePackage.PackageFormat.RPM,
            platform_family=SoftwarePackage.PlatformFamily.RHEL,
            platform_major='9', service_run_as_user='dj-agent',
        )
        upload = SimpleUploadedFile('fluent-bit.deb', b'fake-deb')

        response = self.client.post(
            f'/monitor/packages/{pkg.id}/upload/', {'file': upload}, format='multipart',
        )

        self.assertEqual(response.json()['code'], 400)
        self.assertIn('文件格式', response.json()['msg'])


class SoftwarePackageSelectionTest(TestCase):
    """离线包只能精确匹配 Host 平台族、主版本和架构。"""

    def setUp(self):
        from assets.models import HostHardware, HostSystem

        self.host = Host.objects.create(instance_name='offline-package-host', ip='10.0.9.10')
        self.system = HostSystem.objects.create(
            host=self.host, os_id='centos', os_id_like='rhel fedora', os_version_id='8.5',
        )
        self.hardware = HostHardware.objects.create(host=self.host, architecture='x86_64')

    def _package(self, family, major, arch='amd64', package_format='rpm', package_type='fluent_bit'):
        from .models import SoftwarePackage

        return SoftwarePackage.objects.create(
            package_type=package_type,
            name='fluent-bit', version=f'3.2.{major}', os='linux', arch=arch,
            platform_family=family, platform_major=major, package_format=package_format,
            file=f'monitor_packages/fluent-bit-{major}.{package_format}', enabled=True,
        )

    def test_rhel_family_uses_exact_major_and_architecture(self):
        from .models import SoftwarePackage
        from .package_selector import select_software_package

        self._package(SoftwarePackage.PlatformFamily.RHEL, '7')
        expected = self._package(SoftwarePackage.PlatformFamily.RHEL, '8')
        self._package(SoftwarePackage.PlatformFamily.RHEL, '8', arch='arm64')
        self.assertEqual(select_software_package(
            self.host, 'fluent-bit', SoftwarePackage.PackageType.FLUENT_BIT,
        ).id, expected.id)

    def test_ubuntu_selects_deb_for_its_major_version(self):
        from .models import SoftwarePackage
        from .package_selector import select_software_package

        self.system.os_id = 'ubuntu'
        self.system.os_id_like = 'debian'
        self.system.os_version_id = '22.04'
        self.system.save(update_fields=['os_id', 'os_id_like', 'os_version_id'])
        expected = self._package(
            SoftwarePackage.PlatformFamily.UBUNTU, '22', package_format=SoftwarePackage.PackageFormat.DEB,
        )
        self.assertEqual(select_software_package(
            self.host, 'fluent-bit', SoftwarePackage.PackageType.FLUENT_BIT,
        ).id, expected.id)

    def test_missing_exact_package_does_not_cross_os_version(self):
        from .models import SoftwarePackage
        from .package_selector import PackageSelectionError, select_software_package

        self._package(SoftwarePackage.PlatformFamily.RHEL, '9')
        with self.assertRaisesRegex(PackageSelectionError, 'rhel-8/amd64'):
            select_software_package(
                self.host, 'fluent-bit', SoftwarePackage.PackageType.FLUENT_BIT,
            )

    def test_selection_does_not_cross_package_type(self):
        from .models import SoftwarePackage
        from .package_selector import select_software_package

        exporter = self._package(
            SoftwarePackage.PlatformFamily.RHEL, '8',
            package_type=SoftwarePackage.PackageType.EXPORTER,
        )
        fluent_bit = self._package(
            SoftwarePackage.PlatformFamily.RHEL, '8',
            package_type=SoftwarePackage.PackageType.FLUENT_BIT,
        )
        self.assertEqual(select_software_package(
            self.host, 'fluent-bit', SoftwarePackage.PackageType.EXPORTER,
        ).id, exporter.id)
        self.assertEqual(select_software_package(
            self.host, 'fluent-bit', SoftwarePackage.PackageType.FLUENT_BIT,
        ).id, fluent_bit.id)

    def test_fluent_bit_selection_reports_host_refresh_error_before_unknown_platform(self):
        from .log_collection_service import LogCollectionApplyError, _select_fluent_bit_package

        self.system.os_id = None
        self.system.os_id_like = None
        self.system.os_version_id = None
        self.system.save(update_fields=['os_id', 'os_id_like', 'os_version_id'])
        target = mock.Mock(host=self.host)
        with mock.patch(
            'assets.host_info.refresh_host_info',
            return_value={'updated': False, 'error': 'agent gRPC 通道未连接'},
        ):
            with self.assertRaisesRegex(LogCollectionApplyError, 'agent gRPC 通道未连接') as context:
                _select_fluent_bit_package(target)
        self.assertNotIn('unknown-unknown', str(context.exception))

    def test_fluent_bit_selection_refreshes_missing_platform_before_exact_match(self):
        from .log_collection_service import _select_fluent_bit_package
        from .models import SoftwarePackage

        self.system.os_id = None
        self.system.os_id_like = None
        self.system.os_version_id = None
        self.system.save(update_fields=['os_id', 'os_id_like', 'os_version_id'])
        expected = self._package(SoftwarePackage.PlatformFamily.RHEL, '8')

        def refresh_platform(_host):
            self.system.os_id = 'rhel'
            self.system.os_id_like = 'fedora'
            self.system.os_version_id = '8.9'
            self.system.save(update_fields=['os_id', 'os_id_like', 'os_version_id'])
            return {'updated': True, 'error': ''}

        with mock.patch('assets.host_info.refresh_host_info', side_effect=refresh_platform):
            selected = _select_fluent_bit_package(mock.Mock(host=self.host))
        self.assertEqual(selected.id, expected.id)


class _FakeWriteSession:
    def __init__(self, store, dir_path, filename):
        self._store = store
        self._dir = dir_path
        self._name = filename
        self._buf = b''

    def write_chunk(self, data):
        self._buf += data

    def close(self, abort=False):
        if not abort:
            self._store[f'{self._dir}/{self._name}'] = self._buf.decode('utf-8')


class _FakeListEntry:
    def __init__(self, name):
        self.name = name


class _FakeListResponse:
    def __init__(self, names):
        self.entries = [_FakeListEntry(name) for name in names]


class _FakeGrpcClient:
    """模拟 dj-agent gRPC 通道：记录写入/删除的文件与执行的命令。"""

    def __init__(self, agent_id, **kwargs):
        self.agent_id = agent_id
        self.files = {}
        self.commands = []
        self.deleted = []

    def mkdir(self, path, name):
        return None

    def open_write(self, dir_path, file_name):
        return _FakeWriteSession(self.files, dir_path, file_name)

    def list_dir(self, path):
        prefix = f'{path}/'
        return _FakeListResponse([
            key[len(prefix):] for key in self.files if key.startswith(prefix)
        ])

    def delete(self, path, recursive=False):
        self.deleted.append(path)
        self.files.pop(path, None)

    def execute_automation(self, job_id, params, timeout_seconds, task_type='custom', action='run_automation_task'):
        command = str((params or {}).get('command', ''))
        self.commands.append(command)
        if 'reload' in command:
            return {'status': 'success', 'exit_code': 0, 'stdout': '{"reload":"done"}', 'stderr': ''}
        if action == 'check_exporter_status':
            return {
                'status': 'success', 'exit_code': 0,
                'stdout': '● fluent-bit.service - Fluent Bit\n   Active: active (running)',
                'stderr': '',
            }
        if command.startswith('tail -n'):
            return {'status': 'success', 'exit_code': 0, 'stdout': 'line1\nline2\n', 'stderr': ''}
        return {'status': 'success', 'exit_code': 0, 'stdout': '', 'stderr': ''}


class LogCollectionApplyTest(TestCase):
    """阶段 5 下发链路：指纹幂等、片段写入、热重载、过期片段清理（架构文档 §8.4/§8.5/§8.6）。"""

    def setUp(self):
        self.client = APIClient()
        self.user = SysUser.objects.create(username='admin', password='admin123', status=1)
        self.client.credentials(HTTP_AUTHORIZATION=_get_token(self.user))
        self.cluster = OpenSearchCluster.objects.create(
            name='test', hosts='https://10.25.66.150:9200', index_prefix='logs',
        )
        self.host = Host.objects.create(
            instance_name='log-host-apply', ip='10.0.2.11', agent_id='agent-log-1',
        )
        self.target = LogCollectionTarget.objects.create(
            host=self.host,
            agent_installed=True,
            runtime_status=LogCollectionTarget.RuntimeStatus.RUNNING,
        )
        self._build_service_graph()

    def assertResponseOK(self, res):
        body = res.json()
        self.assertEqual(body['code'], 200, msg=f"Expected code=200, got: {body}")
        return body

    def _build_service_graph(self):
        application = Application.objects.create(name='Tomcat', code='tomcat')
        system = BusinessSystem.objects.create(name='TIB', code='tib')
        environment = BusinessEnvironment.objects.create(name='测试', code='test')
        version = ApplicationVersion.objects.create(application=application, version='9.0.35')
        template = ApplicationDeploymentTemplate.objects.create(
            application=application, name='默认模板', control_type='command',
            run_user='esb', app_home='/home/esb/tomcat',
        )
        rule = LogProcessingRule.objects.create(
            cluster=self.cluster, name='app-tomcat-catalina', multiline_enabled=True,
            start_pattern=r'^\d{4}-\d{2}-\d{2}', continuation_pattern=r'^(?!\d{4}-\d{2}-\d{2})',
            pipeline_body={'processors': []},
        )
        ApplicationLogDefinition.objects.create(
            deployment_template=template, name='catalina',
            path_pattern='${APP_HOME}/logs/catalina.out',
            collection_enabled=True, processing_rule=rule,
        )
        deployment = ApplicationDeployment.objects.create(host=self.host, instance_name='kul-tib-tomcat1')
        service = ApplicationService.objects.create(
            business_system=system, environment=environment, application=application,
            application_version=version, deployment_template=template,
            name='tomcat服务', code='tomcat-svc', log_collection_enabled=True,
        )
        ApplicationServiceDeployment.objects.create(service=service, deployment=deployment)
        self.service = service

    def test_apply_writes_fragments_and_reloads(self):
        with mock.patch('monitor.log_collection_service.AgentChannelClient', _FakeGrpcClient):
            res = self.client.post(f'/monitor/log-targets/{self.target.id}/apply/')
        body = self.assertResponseOK(res)
        self.assertFalse(body['data']['skipped'])
        self.assertIn('tomcat__tomcat-svc__kul-tib-tomcat1__catalina.conf', body['data']['inputs'])
        # 指纹已落库，热重载已触发
        self.target.refresh_from_db()
        self.assertEqual(self.target.config_fingerprint, body['data']['fingerprint'])
        self.assertEqual(self.target.runtime_status, LogCollectionTarget.RuntimeStatus.RUNNING)

    def test_apply_skips_when_fingerprint_unchanged(self):
        with mock.patch('monitor.log_collection_service.AgentChannelClient', _FakeGrpcClient):
            self.client.post(f'/monitor/log-targets/{self.target.id}/apply/')
            res = self.client.post(f'/monitor/log-targets/{self.target.id}/apply/')
        body = self.assertResponseOK(res)
        # 指纹一致直接跳过，不再写文件/重载（§8.4）
        self.assertTrue(body['data']['skipped'])

    def test_apply_cleans_stale_fragments(self):
        fake_client = None

        def _client_factory(agent_id, **kwargs):
            nonlocal fake_client
            if fake_client is None:
                fake_client = _FakeGrpcClient(agent_id)
            return fake_client

        with mock.patch('monitor.log_collection_service.AgentChannelClient', _client_factory):
            self.client.post(f'/monitor/log-targets/{self.target.id}/apply/')
            fake_client.files[
                '/etc/fluent-bit/inputs.d/_djadmin_bootstrap.conf'
            ] = '[INPUT]\n    Name dummy\n'
            fake_client.files[
                '/etc/fluent-bit/outputs.d/_djadmin_bootstrap.conf'
            ] = '[OUTPUT]\n    Name null\n'
            # 关闭采集后目标片段集为空，存量片段必须被删除（§8.6）
            self.service.log_collection_enabled = False
            self.service.save(update_fields=['log_collection_enabled'])
            res = self.client.post(f'/monitor/log-targets/{self.target.id}/apply/')
        self.assertResponseOK(res)
        assert fake_client is not None
        self.assertIn(
            '/etc/fluent-bit/inputs.d/tomcat__tomcat-svc__kul-tib-tomcat1__catalina.conf', fake_client.deleted,
        )
        self.assertIn('/etc/fluent-bit/outputs.d/tomcat__tomcat-svc__catalina.conf', fake_client.deleted)
        self.assertNotIn(
            '/etc/fluent-bit/inputs.d/_djadmin_bootstrap.conf', fake_client.deleted,
        )
        self.assertNotIn(
            '/etc/fluent-bit/outputs.d/_djadmin_bootstrap.conf', fake_client.deleted,
        )

    def test_apply_rejects_unbound_agent(self):
        self.host.agent_id = None
        self.host.save(update_fields=['agent_id'])
        res = self.client.post(f'/monitor/log-targets/{self.target.id}/apply/')
        self.assertEqual(res.json()['code'], 400)
        self.assertIn('agent', res.json()['msg'])

    def test_apply_rejects_uninstalled_fluent_bit_before_file_write(self):
        self.target.agent_installed = False
        self.target.runtime_status = LogCollectionTarget.RuntimeStatus.UNKNOWN
        self.target.save(update_fields=['agent_installed', 'runtime_status'])
        with mock.patch('monitor.log_collection_service.AgentChannelClient') as client_class:
            res = self.client.post(f'/monitor/log-targets/{self.target.id}/apply/')
        self.assertEqual(res.json()['code'], 400)
        self.assertIn('尚未安装', res.json()['msg'])
        client_class.assert_not_called()

    def test_check_status_updates_target(self):
        with mock.patch('monitor.log_collection_service.AgentChannelClient', _FakeGrpcClient):
            res = self.client.post(f'/monitor/log-targets/{self.target.id}/check-status/')
        body = self.assertResponseOK(res)
        self.assertTrue(body['data']['running'])
        self.target.refresh_from_db()
        self.assertTrue(self.target.agent_installed)
        self.assertEqual(self.target.install_status, LogCollectionTarget.InstallStatus.SUCCESS)
        self.assertEqual(self.target.runtime_status, LogCollectionTarget.RuntimeStatus.RUNNING)

    def test_check_status_returns_business_error_when_agent_channel_is_disconnected(self):
        from assets.grpc_transfer.client import AgentGrpcTransferError

        with mock.patch(
            'monitor.log_collection_service.AgentChannelClient',
            side_effect=AgentGrpcTransferError('agent gRPC 通道未连接'),
        ):
            res = self.client.post(f'/monitor/log-targets/{self.target.id}/check-status/')
        self.assertEqual(res.json()['code'], 400)
        self.assertIn('gRPC 通道未连接', res.json()['msg'])
        self.target.refresh_from_db()
        self.assertIn('gRPC 通道未连接', self.target.last_error)

    def test_log_tail_reads_instance_log(self):
        with mock.patch('monitor.log_collection_service.AgentChannelClient', _FakeGrpcClient):
            res = self.client.get(
                f'/monitor/log-targets/{self.target.id}/log-tail/'
                '?instance_name=kul-tib-tomcat1&log_name=catalina&lines=50'
            )
        body = self.assertResponseOK(res)
        self.assertEqual(body['data']['log_path'], '/home/esb/tomcat/logs/catalina.out')
        self.assertIn('line1', body['data']['content'])
        # 行数经服务端收敛，不接受超大值
        self.assertLessEqual(body['data']['lines'], 1000)

    def test_log_tail_rejects_unknown_instance(self):
        with mock.patch('monitor.log_collection_service.AgentChannelClient', _FakeGrpcClient):
            res = self.client.get(
                f'/monitor/log-targets/{self.target.id}/log-tail/'
                '?instance_name=not-exist&log_name=catalina'
            )
        self.assertEqual(res.json()['code'], 400)


class LogCollectionLifecycleApiTest(TestCase):
    """Fluent Bit 与 Exporter 对齐的安装、服务控制、取消、日志和安全删除动作。"""

    def setUp(self):
        self.client = APIClient()
        self.user = SysUser.objects.create(username='admin', password='admin123', status=1)
        self.client.credentials(HTTP_AUTHORIZATION=_get_token(self.user))
        self.host = Host.objects.create(
            instance_name='fluent-lifecycle-host', ip='10.0.3.11', agent_id='agent-fluent-lifecycle',
        )
        self.target = LogCollectionTarget.objects.create(host=self.host)

    def assertResponseOK(self, response):
        body = response.json()
        self.assertEqual(body['code'], 200, msg=f'Expected code=200, got: {body}')
        self.assertIn('msg', body)
        self.assertIn('data', body)
        return body

    def test_retry_dispatches_fluent_bit_install(self):
        with mock.patch('monitor.views.dispatch_fluent_bit_install') as dispatch:
            response = self.client.post(f'/monitor/log-targets/{self.target.id}/retry/')
        self.assertResponseOK(response)
        dispatch.assert_called_once()
        self.assertEqual(dispatch.call_args.args[0].id, self.target.id)
        self.assertTrue(dispatch.call_args.kwargs['manual'])

    def test_retry_rejects_duplicate_active_task(self):
        from .models import MonitorTargetInstallHistory

        MonitorTargetInstallHistory.objects.create(
            log_collection_target=self.target,
            host=self.host,
            action=MonitorTargetInstallHistory.Action.INSTALL,
            status=MonitorTargetInstallHistory.Status.PENDING,
            exporter_type_snapshot='fluent-bit',
        )
        response = self.client.post(f'/monitor/log-targets/{self.target.id}/retry/')
        self.assertEqual(response.json()['code'], 400)
        self.assertIn('正在执行', response.json()['msg'])

    def test_list_uses_grpc_registry_as_agent_online_source(self):
        self.host.agent_online = True
        self.host.save(update_fields=['agent_online'])
        with mock.patch('monitor.views.REGISTRY.connected_agent_ids', return_value=[]):
            response = self.client.get('/monitor/log-targets/')
        body = self.assertResponseOK(response)
        self.host.refresh_from_db()
        self.assertFalse(self.host.agent_online)
        self.assertFalse(body['data']['results'][0]['host_agent_online'])

    def test_start_and_stop_service_update_runtime_status(self):
        self.target.agent_installed = True
        self.target.save(update_fields=['agent_installed'])
        with mock.patch('monitor.log_collection_service.AgentChannelClient', _FakeGrpcClient):
            self.assertResponseOK(self.client.post(f'/monitor/log-targets/{self.target.id}/start-service/'))
            self.target.refresh_from_db()
            self.assertEqual(self.target.runtime_status, LogCollectionTarget.RuntimeStatus.RUNNING)
            self.assertResponseOK(self.client.post(f'/monitor/log-targets/{self.target.id}/stop-service/'))
        self.target.refresh_from_db()
        self.assertEqual(self.target.runtime_status, LogCollectionTarget.RuntimeStatus.STOPPED)

    def test_cancel_only_updates_latest_fluent_bit_history(self):
        from .models import MonitorTargetInstallHistory

        history = MonitorTargetInstallHistory.objects.create(
            log_collection_target=self.target,
            host=self.host,
            action=MonitorTargetInstallHistory.Action.INSTALL,
            status=MonitorTargetInstallHistory.Status.RUNNING,
            exporter_type_snapshot='fluent-bit',
            start_time=timezone.now(),
        )
        self.target.install_status = LogCollectionTarget.InstallStatus.PENDING
        self.target.save(update_fields=['install_status'])

        response = self.client.post(f'/monitor/log-targets/{self.target.id}/cancel/')
        self.assertResponseOK(response)
        history.refresh_from_db()
        self.target.refresh_from_db()
        self.assertEqual(history.status, MonitorTargetInstallHistory.Status.CANCELLED)
        self.assertEqual(self.target.install_status, LogCollectionTarget.InstallStatus.FAILED)

    def test_history_filter_and_generic_cancel_support_fluent_bit_target(self):
        from .models import MonitorTargetInstallHistory

        history = MonitorTargetInstallHistory.objects.create(
            log_collection_target=self.target,
            host=self.host,
            action=MonitorTargetInstallHistory.Action.INSTALL,
            status=MonitorTargetInstallHistory.Status.PENDING,
            exporter_type_snapshot='fluent-bit',
        )
        list_response = self.client.get(
            f'/monitor/install-histories/?log_collection_target_id={self.target.id}'
        )
        list_body = self.assertResponseOK(list_response)
        self.assertEqual([row['id'] for row in list_body['data']['results']], [history.id])

        cancel_response = self.client.post(f'/monitor/install-histories/{history.id}/cancel/')
        self.assertResponseOK(cancel_response)
        self.target.refresh_from_db()
        self.assertEqual(self.target.install_status, LogCollectionTarget.InstallStatus.UNKNOWN)

    def test_delete_uninstalled_target_without_dispatch(self):
        response = self.client.delete(f'/monitor/log-targets/{self.target.id}/')
        self.assertResponseOK(response)
        self.assertFalse(LogCollectionTarget.objects.filter(id=self.target.id).exists())

    def test_delete_keeps_target_when_uninstall_fails(self):
        from .models import MonitorTargetInstallHistory

        self.target.agent_installed = True
        self.target.save(update_fields=['agent_installed'])
        failed_history = MonitorTargetInstallHistory(
            status=MonitorTargetInstallHistory.Status.FAILED,
        )
        with mock.patch('monitor.views.dispatch_fluent_bit_uninstall', return_value=failed_history):
            response = self.client.delete(f'/monitor/log-targets/{self.target.id}/')
        self.assertEqual(response.json()['code'], 400)
        self.assertTrue(LogCollectionTarget.objects.filter(id=self.target.id).exists())

