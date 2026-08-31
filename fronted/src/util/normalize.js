export function asArray(value) {
  if (Array.isArray(value)) return value.filter((item) => item !== null && item !== undefined)
  return []
}

export function asObject(value, fallback = {}) {
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    return value
  }
  return fallback
}
