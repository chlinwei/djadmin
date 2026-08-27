"""离线软件包选择。

Host 不访问互联网；djadmin 从本地仓库中按 /etc/os-release 与 CPU 架构选择唯一适配包，
再沿现有 Ansible/SSH 链路传输。rpm/deb 禁止跨发行版或跨主版本兜底安装。
"""
from .models import SoftwarePackage


_ARCH_ALIASES = {
    'x86_64': SoftwarePackage.ArchType.AMD64,
    'amd64': SoftwarePackage.ArchType.AMD64,
    'aarch64': SoftwarePackage.ArchType.ARM64,
    'arm64': SoftwarePackage.ArchType.ARM64,
}

_RHEL_IDS = {'rhel', 'centos', 'rocky', 'almalinux', 'ol'}


class PackageSelectionError(ValueError):
    pass


def normalize_host_platform(host):
    """将 Agent 上报的 os-release 与 uname 架构归一为仓库匹配键。"""
    system = getattr(host, 'system', None)
    hardware = getattr(host, 'hardware', None)
    os_id = str(getattr(system, 'os_id', '') or '').strip().lower()
    os_id_like = set(str(getattr(system, 'os_id_like', '') or '').strip().lower().split())
    version_id = str(getattr(system, 'os_version_id', '') or '').strip()
    architecture = str(getattr(hardware, 'architecture', '') or '').strip().lower()

    if os_id in _RHEL_IDS or os_id_like.intersection(_RHEL_IDS):
        family = SoftwarePackage.PlatformFamily.RHEL
    elif os_id == 'ubuntu':
        family = SoftwarePackage.PlatformFamily.UBUNTU
    elif os_id == 'debian' or 'debian' in os_id_like:
        family = SoftwarePackage.PlatformFamily.DEBIAN
    else:
        family = ''

    major = version_id.split('.', 1)[0] if version_id else ''
    arch = _ARCH_ALIASES.get(architecture, '')
    return {'family': family, 'major': major, 'arch': arch}


def select_software_package(host, package_name, package_type):
    """选择适合 Host 的最新启用包。

    通用 tar.gz 使用 platform_family=any；rpm/deb 必须精确匹配平台族与主版本。
    两条都是显式包类型，不进行跨平台或跨版本回退。
    """
    queryset = SoftwarePackage.objects.filter(
        name=package_name,
        package_type=package_type,
        enabled=True,
    ).exclude(file='')
    if not queryset.exists():
        raise PackageSelectionError(
            f'本地软件仓库缺少 {package_name} 的启用 {package_type} 安装包，请先上传对应的离线安装包'
        )

    platform = normalize_host_platform(host)
    if not platform['arch']:
        raise PackageSelectionError('主机架构信息缺失，请先执行资产采集')
    queryset = queryset.filter(arch=platform['arch'])

    expected_formats = {
        SoftwarePackage.PlatformFamily.RHEL: (SoftwarePackage.PackageFormat.RPM,),
        SoftwarePackage.PlatformFamily.UBUNTU: (SoftwarePackage.PackageFormat.DEB,),
        SoftwarePackage.PlatformFamily.DEBIAN: (SoftwarePackage.PackageFormat.DEB,),
    }.get(platform['family'])
    exact = queryset.filter(
        platform_family=platform['family'],
        platform_major=platform['major'],
        package_format__in=expected_formats or (),
    ).order_by('-create_time').first()
    if exact is not None:
        return exact

    portable = queryset.filter(
        package_format=SoftwarePackage.PackageFormat.TAR_GZ,
        platform_family=SoftwarePackage.PlatformFamily.ANY,
        platform_major='',
    ).order_by('-create_time').first()
    if portable is not None:
        return portable

    family = platform['family'] or 'unknown'
    major = platform['major'] or 'unknown'
    raise PackageSelectionError(
        f'本地软件仓库缺少 {package_name} 的 {family}-{major}/{platform["arch"]} 安装包；'
        '请先上传匹配的离线 rpm/deb 包'
    )
