export const getHostDisplayName = (host) => {
  const instanceName = String(host?.instance_name || '').trim()
  const hostname = String(host?.system?.hostname || '').trim()
  const ip = String(host?.ip || '').trim()
  return instanceName || hostname || ip || `Host-${host?.id || '-'}`
}
