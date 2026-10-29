export function asArray(value) {
  if (Array.isArray(value)) return value.filter((item) => item !== null && item !== undefined)
  return []
}
