// 日志中心「非服务节点」的层级视图：把服务树的一个层级（全部/项目/业务系统/环境）折叠成
// "下一层清单 + 指标条"。
//
// 与「存储水位」tab 的 groupDimension 是同一套层级映射（全部→项目、项目→业务系统、
// 业务系统→环境、环境→逻辑服务），只是口径不同：那边是磁盘占用与文档数，这里是**日志配置状态**
//（几条日志、几条没认证、几台待下发、采集开没开）。
//
// 为什么抽成纯函数：层级折叠最容易出错的地方是"上层数字和下钻后看到的对不上"。这里的每一行
// 都由**同一批服务行**聚合而来（上层的行 = 它下面那些服务行之和），所以两边必然一致；
// 把这套聚合放在可以直测的地方，比散在组件里靠肉眼对数字可靠。

// LEVEL_DIMENSIONS 每个层级节点的"下一层"是什么。与资产服务树页的面包屑/子表层级一致：
// 业务系统下先按环境收拢（同一环境下可能挂多个服务），再往下才是服务。
const LEVEL_DIMENSIONS = {
  all: { key: 'project', label: '项目' },
  project: { key: 'businessSystem', label: '业务系统' },
  businessSystem: { key: 'environment', label: '环境' },
  environment: { key: 'service', label: '逻辑服务' },
}

// UNSASSIGNED 环境/项目的兜底分组（服务没配环境时也要能列出来，否则这些服务在层级视图里会消失）。
const UNASSIGNED = 'unassigned'

// levelDimension 返回某层级节点的下一层维度；服务/实例层返回 null（那是服务级界面，不是层级视图）。
export function levelDimension(nodeType) {
  return LEVEL_DIMENSIONS[nodeType] || null
}

function serviceEnvironmentId(service) {
  return service.environment ?? UNASSIGNED
}

function serviceEnvironmentName(service) {
  return service.environment_name || '未配置环境'
}

// buildLevelRows 把一个层级折叠成清单行。
//
// 入参：
// - scope：服务树选中的节点（nodeType 决定这一层看什么；project/businessSystem/environment 层用它收窄）
// - projects / businessSystems / services：资产侧的全量名单（服务行带 business_system / environment /
//   log_collection_enabled / log_retention_tier_id —— 采集开关与档位的权威值就在这些行上）
// - statusByService：服务 id → 日志状态汇总（后端 log-status-summary 的 items，可能为空）
// - usageByService：服务**编码** → 最近写入文档数（只有环境层会传，见 getLogServiceUsage）
//
// 每行的数字 = 它下面那些服务的和；服务行的数字 = 它自己。pending（待下发）单位是 (服务 × 主机)。
export function buildLevelRows({ scope, projects = [], businessSystems = [], services = [], statusByService = {}, usageByService = {} } = {}) {
  const dimension = levelDimension(scope?.nodeType)
  if (!dimension) return { dimension: null, rows: [] }

  // 根节点上方的"项目 / 环境"多选是这一层的过滤器（与存储水位 tab 同一口径）。
  // 只在根节点上成立：下钻到具体节点后，scope 里带的是那个节点的 id，不再有这两个数组。
  const scopedServices = filterServicesByRootFilters(services, scope)
  const members = levelMembers(scope, dimension.key, projects, businessSystems, scopedServices)
  const rows = members.map((member) => {
    const memberServices = servicesUnder(member, dimension.key, scopedServices, businessSystems)
    return summarizeMember(member, memberServices, statusByService, usageByService, dimension.key)
  })
  return { dimension, rows }
}

// filterServicesByRootFilters 应用根节点上方的**环境**多选。
//
// 为什么必须过滤：不过滤的话，用户在树上方筛了"只看某个环境"，这一层却仍然把全部服务算进指标条
// 与清单 —— 界面上出现"筛选了但数字没变"。存储水位 tab 一直是这么做的（scopeCodes）。
// 项目多选不在这里处理：它由 levelMembers 过滤掉未选中的项目行即可（每行的服务都经该行自己的
// 业务系统解析出来，未选中项目的服务不会进任何一行）。
function filterServicesByRootFilters(services, scope) {
  if (!scope || scope.nodeType !== 'all') return services
  const environmentIds = Array.isArray(scope.environmentIds) ? scope.environmentIds : []
  if (!environmentIds.length) return services
  const allowed = new Set(environmentIds)
  return services.filter((service) => allowed.has(service.environment))
}

// levelMembers 这一层要列出的成员（下一层的节点）。
//
// 每一层都要**按 scope 收窄**：漏了收窄就会出现"点某个项目，却列出全部业务系统"（现场反馈），
// 或者把隔壁业务系统的服务算进这一层的数字里 —— 两者都是同一类错误。
function levelMembers(scope, dimensionKey, projects, businessSystems, services) {
  if (dimensionKey === 'project') {
    // 根节点：项目多选生效时只看选中的项目。
    const allowed = new Set(Array.isArray(scope?.projectIds) ? scope.projectIds : [])
    return projects.filter((project) => !allowed.size || allowed.has(project.id)).map((project) => ({
      key: `project:${project.id}`,
      title: project.name,
      scope: { nodeType: 'project', projectId: project.id, nodeTitle: project.name },
    }))
  }
  if (dimensionKey === 'businessSystem') {
    // **项目节点只列这个项目的业务系统**（业务系统上的 project 字段是权威归属）。
    return businessSystemsInProject(businessSystems, scope?.projectId).map((system) => ({
      key: `system:${system.id}`,
      title: system.name,
      scope: { nodeType: 'businessSystem', businessSystemId: system.id, nodeTitle: system.name },
    }))
  }
  if (dimensionKey === 'environment') {
    // 环境层不是一张独立的表：把该业务系统下的服务按环境收拢。
    const environments = new Map()
    for (const service of servicesInBusinessSystem(services, scope.businessSystemId)) {
      const environmentId = serviceEnvironmentId(service)
      if (!environments.has(environmentId)) {
        const name = serviceEnvironmentName(service)
        environments.set(environmentId, {
          key: `environment:${scope.businessSystemId}:${environmentId}`,
          title: name,
          scope: {
            nodeType: 'environment', businessSystemId: scope.businessSystemId,
            businessSystemName: scope.nodeTitle, environment: environmentId === UNASSIGNED ? null : environmentId,
            environmentName: name, nodeTitle: name,
          },
        })
      }
    }
    return [...environments.values()]
  }
  // 叶子层：就是服务自己。点行 = 进入该服务的日志配置/查询。
  return servicesInEnvironment(services, scope).map((service) => ({
    key: `service:${service.id}`,
    title: service.name,
    service,
    scope: {
      nodeType: 'service', applicationServiceId: service.id, nodeTitle: service.name,
      businessSystemId: service.business_system, environment: service.environment ?? null,
      environmentName: serviceEnvironmentName(service),
    },
  }))
}

function servicesInBusinessSystem(services, businessSystemId) {
  if (businessSystemId === undefined || businessSystemId === null) return services
  return services.filter((service) => service.business_system === businessSystemId)
}

// businessSystemsInProject 项目的业务系统归属以**业务系统行上的 project 字段**为准。
// 别用"这个项目下有没有服务"反推：那样没建服务的业务系统会整行消失（它们在层级视图里必须可见，
// 否则用户看到的是"这个项目什么都没有"，而实际是"业务系统建了但还没建服务"）。
function businessSystemsInProject(businessSystems, projectId) {
  if (projectId === undefined || projectId === null) return businessSystems
  return businessSystems.filter((system) => system.project === projectId)
}

function servicesInEnvironment(services, scope) {
  const environmentId = scope.environment ?? UNASSIGNED
  return servicesInBusinessSystem(services, scope.businessSystemId)
    .filter((service) => serviceEnvironmentId(service) === environmentId)
}

// servicesUnder 一行下面挂着哪些服务（上层行 = 下钻后能看到的那些服务，保证上下层数字一致）。
function servicesUnder(member, dimensionKey, services, businessSystems) {
  if (dimensionKey === 'service') return member.service ? [member.service] : []
  if (dimensionKey === 'environment') {
    // 与 levelMembers 同一套收窄：环境是业务系统内的概念，只看环境 id 会串到别的业务系统。
    return servicesInEnvironment(services, {
      businessSystemId: member.scope.businessSystemId,
      environment: member.scope.environment,
    })
  }
  if (dimensionKey === 'businessSystem') {
    return servicesInBusinessSystem(services, member.scope.businessSystemId)
  }
  // project 层：服务 → 业务系统 → 项目（服务行上没有项目字段，所以经业务系统折一层）。
  const systemIds = new Set(businessSystems.filter((system) => system.project === member.scope.projectId).map((system) => system.id))
  return services.filter((service) => systemIds.has(service.business_system))
}

function summarizeMember(member, memberServices, statusByService, usageByService, dimensionKey) {
  const row = {
    key: member.key,
    title: member.title,
    scope: member.scope,
    isService: dimensionKey === 'service',
    serviceCount: memberServices.length,
    // 这一行下面挂着哪些服务：调用方拿它去问日志状态汇总（只问范围内实际列出的服务）。
    serviceIds: memberServices.map((service) => service.id),
    collectionOff: 0,
    // collectionOn：**可查的服务数**（采集总开关开着）。查询 tab 的"能不能查"就看这个——
    // 没开采集的服务查不到日志，这是"查不到"的头号原因。
    collectionOn: 0,
    logs: 0,
    verified: 0,
    needsRecheck: 0,
    unverified: 0,
    disabledLogs: 0,
    noRule: 0,
    pendingPairs: 0,
    managedPairs: 0,
    unmanagedPairs: 0,
    // recentDocs 只在叶子层有值（其他层要算就得按服务逐个查 ES，代价不值）；null = 没这项数据。
    recentDocs: null,
    recentDocsKnown: false,
    // hasRecentDocs：窗口内是否有写入。null = "不知道"（取不到写入量），与"知道但没有"（false）不同。
    hasRecentDocs: null,
    retentionTierId: null,
    // 配置态是否算出来了：没算出来时这一列显示"-"（后端 pending 为 null / pending_error 非空）。
    pendingKnown: false,
  }
  let usageTotal = 0
  let usageKnown = false
  for (const service of memberServices) {
    if (service.log_collection_enabled === false) {
      row.collectionOff += 1
    } else {
      row.collectionOn += 1
    }
    const status = statusByService[service.id]
    if (status) {
      row.logs += status.logs || 0
      row.verified += status.verified || 0
      row.needsRecheck += status.needs_recheck || 0
      row.unverified += status.unverified || 0
      row.disabledLogs += status.disabled_logs || 0
      row.noRule += status.no_rule || 0
      if (status.pending) {
        row.pendingKnown = true
        row.pendingPairs += (status.pending.drift || 0) + (status.pending.never || 0)
        row.managedPairs += status.pending.managed || 0
        row.unmanagedPairs += status.pending.unmanaged || 0
      }
    }
    if (dimensionKey === 'service') row.retentionTierId = service.log_retention_tier_id ?? null
    if (usageByService && Object.prototype.hasOwnProperty.call(usageByService, service.code)) {
      usageKnown = true
      usageTotal += Number(usageByService[service.code] || 0)
    }
  }
  if (dimensionKey === 'service') {
    row.recentDocsKnown = usageKnown
    row.recentDocs = usageKnown ? usageTotal : null
    // hasRecentDocs 是"查询 tab 看有没有数据"的正向信号：只有真的拿到写入量（usageKnown）才谈得上
    // "有/没有"——拿不到数据时是"不知道"，不能算成"没有写入"。
    row.hasRecentDocs = usageKnown ? usageTotal > 0 : null
  }
  return row
}

// buildLevelMetrics 指标条：对清单行求和。**口径与清单里的数字同一来源**，
// 所以"指标条说 3 条未认证、点进去逐行加起来也是 3 条"不会出现两个答案。
//
// 两个 tab 各取所需（见 LogLevelOverview 的 variant）：
//   - 配置 tab 看欠账：unverified / needsRecheck / pendingPairs；
//   - 查询 tab 看能不能查、有没有数据：collectionOn / collectionOff / recentServices。
export function buildLevelMetrics(rows = []) {
  const metrics = {
    services: 0, collectionOn: 0, collectionOff: 0,
    unverified: 0, needsRecheck: 0, pendingPairs: 0, managedPairs: 0, unmanagedPairs: 0,
    // 待下发的口径是 (服务 × 主机)：同一台主机上的两个服务各算一次，这才是"下发这件事"的粒度。
    pendingKnown: rows.some((row) => row.pendingKnown),
    // 查询视角：窗口内有写入的服务数 / 无写入的服务数。**只有真的取到写入量才算**
    //（上层算不出这个数，见文件头与文档 §9.5.2 的取舍），否则整组为 null，界面显示"-"。
    recentKnown: rows.some((row) => row.isService && row.recentDocsKnown),
    recentServices: 0,
    recentSilentServices: 0,
  }
  for (const row of rows) {
    metrics.services += row.serviceCount
    metrics.collectionOff += row.collectionOff
    metrics.collectionOn += row.collectionOn
    metrics.unverified += row.unverified
    metrics.needsRecheck += row.needsRecheck
    metrics.pendingPairs += row.pendingPairs
    metrics.managedPairs += row.managedPairs
    metrics.unmanagedPairs += row.unmanagedPairs
    if (row.isService && row.recentDocsKnown) {
      if (row.hasRecentDocs) metrics.recentServices += 1
      else metrics.recentSilentServices += 1
    }
  }
  return metrics
}
