from collections import defaultdict

from django.core.management.base import BaseCommand

from assets.models import ApplicationServiceDeployment, BusinessSystem, Host


class Command(BaseCommand):
    help = 'Audit hosts for single-business ownership violations.'

    def add_arguments(self, parser):
        parser.add_argument(
            '--only-violations',
            action='store_true',
            help='Only print hosts with ownership violations.',
        )

    def handle(self, *args, **options):
        only_violations = bool(options.get('only_violations'))

        host_map = {
            host.id: host
            for host in Host.objects.select_related('environment__business_system').all()
        }
        business_name_map = {
            item.id: item.name
            for item in BusinessSystem.objects.all().only('id', 'name')
        }

        service_business_by_host = defaultdict(set)
        for row in ApplicationServiceDeployment.objects.select_related(
            'deployment__host',
            'service__business_system',
        ).all():
            host_id = row.deployment.host_id
            business_id = row.service.business_system_id
            if host_id and business_id:
                service_business_by_host[host_id].add(business_id)

        violations = []
        rows = []

        for host_id, host in host_map.items():
            business_ids = sorted(service_business_by_host.get(host_id, set()))
            env_business_id = getattr(getattr(host, 'environment', None), 'business_system_id', None)

            flags = []
            if len(business_ids) > 1:
                flags.append('MULTI_BUSINESS_DEPLOYMENTS')
            if business_ids and env_business_id is None:
                flags.append('ENV_MISSING')
            if env_business_id is not None and business_ids and env_business_id not in business_ids:
                flags.append('ENV_BUSINESS_MISMATCH')

            row = {
                'host_id': host_id,
                'host_name': str(host.instance_name or f'Host-{host_id}'),
                'host_ip': str(host.ip or ''),
                'environment': str(getattr(getattr(host, 'environment', None), 'name', '') or '-'),
                'env_business': business_name_map.get(env_business_id, '-') if env_business_id else '-',
                'service_businesses': ', '.join(
                    business_name_map.get(item_id, str(item_id))
                    for item_id in business_ids
                ) or '-',
                'flags': ','.join(flags) or 'OK',
            }

            rows.append(row)
            if flags:
                violations.append(row)

        output_rows = violations if only_violations else rows
        if not output_rows:
            self.stdout.write(self.style.SUCCESS('No ownership violations found.'))
            return

        self.stdout.write(
            'host_id\thost_name\thost_ip\tenvironment\tenv_business\tservice_businesses\tflags'
        )
        for row in output_rows:
            self.stdout.write(
                f"{row['host_id']}\t{row['host_name']}\t{row['host_ip']}\t"
                f"{row['environment']}\t{row['env_business']}\t"
                f"{row['service_businesses']}\t{row['flags']}"
            )

        if violations:
            self.stdout.write(
                self.style.WARNING(
                    f'\nOwnership violations: {len(violations)} / total hosts: {len(rows)}'
                )
            )
        else:
            self.stdout.write(self.style.SUCCESS('\nAll hosts comply with single-business ownership.'))
