export async function copyTextWithFallback(text, env = { navigator, document }) {
  const content = String(text || '')
  if (!content) {
    return true
  }

  const targetNavigator = env?.navigator
  const targetDocument = env?.document

  if (targetNavigator?.clipboard?.writeText) {
    try {
      await targetNavigator.clipboard.writeText(content)
      return true
    } catch (error) {
      // Continue with legacy fallback for restricted clipboard environments.
    }
  }

  if (targetDocument?.createElement && targetDocument?.body?.appendChild && targetDocument?.body?.removeChild && targetDocument?.execCommand) {
    try {
      const textarea = targetDocument.createElement('textarea')
      textarea.value = content
      textarea.setAttribute('readonly', '')
      textarea.style.position = 'fixed'
      textarea.style.opacity = '0'
      targetDocument.body.appendChild(textarea)
      textarea.select()
      textarea.setSelectionRange(0, textarea.value.length)
      const copied = targetDocument.execCommand('copy')
      targetDocument.body.removeChild(textarea)
      return Boolean(copied)
    } catch (error) {
      return false
    }
  }

  return false
}
