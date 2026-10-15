import { buildLevelRows } from './logLevelOverview'

function topRows(rows, field) {
  const sorted = rows.map((row) => ({ name: row.title, value: row[field] }))
    .filter((row) => row.value > 0).sort((a, b) => b.value - a.value)
  if (sorted.length <= 8) return sorted
  return [...sorted.slice(0, 8), {
    name: '其他', value: sorted.slice(8).reduce((sum, row) => sum + row.value, 0),
  }]
}

export function buildServiceTreeResources({ scope, projects, businessSystems, services, deployments, hosts }) {
  const { dimension, rows } = buildLevelRows({ scope, projects, businessSystems, services })
  const hostById = new Map(hosts.map((host) => [String(host.id), host]))
  const totals = rows.map((row) => {
    const serviceIds = new Set(row.serviceIds.map(String))
    const hostIds = new Set(deployments.filter((deployment) => (
      (deployment.application_service_ids || []).some((id) => serviceIds.has(String(id)))
    )).map((deployment) => String(deployment.host_id ?? deployment.host)))
    let cpu = 0
    let memory = 0
    // 同组内一台主机只计一次；跨组容量允许重叠，不将其解释为服务配额。
    for (const id of hostIds) {
      const host = hostById.get(id)
      if (!host) continue
      cpu += Number(host.hardware?.cpu_cores ?? host.cpu_cores ?? 0) || 0
      memory += Number(host.hardware?.memory_gb ?? host.memory_gb ?? 0) || 0
    }
    return { title: row.title, cpu, memory }
  })
  return {
    dimension: dimension?.key,
    cpuRows: topRows(totals, 'cpu'),
    memoryRows: topRows(totals, 'memory'),
    totalCpu: totals.reduce((sum, row) => sum + row.cpu, 0),
    totalMemory: totals.reduce((sum, row) => sum + row.memory, 0),
  }
}
