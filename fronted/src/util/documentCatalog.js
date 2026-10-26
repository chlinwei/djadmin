export const DOCUMENT_HOME = 'docs/README.md'
const modulePath = (name) => `docs/architecture/modules/${name}.md`
const page = (title, name, anchor = '') => ({
  title,
  key: `${modulePath(name)}${anchor ? `#${anchor}` : ''}`,
})
const group = (key, title, children) => ({ key, title, selectable: false, children })

// The reading hierarchy follows product menus, not filenames or implementation packages.
export const documentCatalog = [
  { key: DOCUMENT_HOME, title: '文档中心首页' },
  page('登录认证与权限', 'ACCESS'),
  page('首页', 'HOME'),
  group('assets', '资产管理', [
    page('主机管理与 Agent', 'ASSETS', 'hosts'),
    page('项目管理', 'ASSETS', 'projects'),
    page('环境管理', 'ASSETS', 'environments'),
    page('凭据管理', 'ASSETS', 'credentials'),
    page('应用管理', 'ASSETS', 'applications'),
    page('服务树管理', 'ASSETS', 'service-tree'),
  ]),
  group('automation', '自动化运维', [
    page('模板', 'AUTOMATION', 'templates'),
    page('Inventory 管理', 'AUTOMATION', 'inventory'),
    page('自动化任务', 'AUTOMATION', 'tasks'),
    page('运行记录中心', 'AUTOMATION', 'jobs'),
    page('巡检中心', 'INSPECTION'),
  ]),
  group('monitor', '监控中心', [
    group('alerting', '监控告警', [
      page('智能监控', 'MONITORING', 'dashboard'),
      page('告警规则', 'MONITORING', 'rules'),
      page('告警', 'MONITORING', 'alerts'),
      page('Explore', 'MONITORING', 'explore'),
    ]),
    group('notification', '通知管理', [
      page('媒介', 'MONITORING', 'media'),
      page('通知策略', 'MONITORING', 'policies'),
    ]),
    group('logging', '日志管理', [
      page('日志采集', 'LOGGING', 'collectors'),
      page('日志存储', 'LOGGING', 'storage'),
      page('日志处理规则', 'LOGGING', 'parsers'),
      page('日志保留档位', 'LOGGING', 'retention'),
      group('log-center', '日志中心', [
        page('概览与日志排查', 'LOGGING', 'center'),
        page('配置指纹与待下发排查', 'LOGGING', 'fingerprints'),
      ]),
    ]),
  ]),
  group('security', '安全中心', [page('基线扫描', 'SECURITY')]),
  group('system', '系统管理', [
    page('用户管理', 'SYSTEM', 'users'),
    page('用户组', 'SYSTEM', 'user-groups'),
    page('角色管理', 'SYSTEM', 'roles'),
    page('菜单管理', 'SYSTEM', 'menus'),
    page('系统参数', 'SYSTEM', 'config'),
    page('定时任务', 'SYSTEM', 'scheduler'),
    page('操作日志与登录日志', 'SYSTEM', 'audit'),
  ]),
  page('个人中心', 'PROFILE'),
  page('文档中心', 'DOCUMENTATION'),
]

export function flattenDocumentCatalog(nodes = documentCatalog) {
  return nodes.flatMap((node) => node.children ? flattenDocumentCatalog(node.children) : [node])
}

export function documentAncestorKeys(key, nodes = documentCatalog, parents = []) {
  for (const node of nodes) {
    if (node.key === key) return parents
    if (node.children) {
      const found = documentAncestorKeys(key, node.children, [...parents, node.key])
      if (found) return found
    }
  }
  return null
}
