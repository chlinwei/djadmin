export function baselineAssertions(actual) {
  const details = actual?.details
  if (!Array.isArray(details)) return []
  return details.filter((detail) => detail?.property === 'assertion')
    .map((detail, idx) => ({
      idx,
      title: detail.title || detail.name || '',
      expected: detail.expected,
      actual: detail.actual,
      successful: detail.successful,
      message: detail.message || '',
    }))
}

export function formatEvidence(value) {
  if (value == null) return '—'
  return typeof value === 'string' ? value : JSON.stringify(value, null, 2)
}
