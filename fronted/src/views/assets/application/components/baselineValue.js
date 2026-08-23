export function formatCheckValue(value) {
  if (value === null || value === undefined || value === '') return '-'
  return typeof value === 'object' ? JSON.stringify(value, null, 2) : String(value)
}

export function formatActualCheckValue(result) {
  const value = result?.actual_value
  if (result?.check_type !== 'shell' || !value || typeof value !== 'object') return formatCheckValue(value)
  const stdout = String(value.stdout || '').trim()
  if (stdout) return stdout
  const stderr = String(value.stderr || '').trim()
  if (stderr) return stderr
  return value.exit_code === null || value.exit_code === undefined ? '-' : `退出码 ${value.exit_code}`
}