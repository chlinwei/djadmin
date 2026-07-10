import { test, expect } from '@playwright/test'

async function expectNotRedirectedToLogin(page) {
  const pathname = await page.evaluate(() => window.location.pathname)
  expect(pathname).not.toBe('/login')
}

async function openAndWait(page, path) {
  await page.goto(path, { waitUntil: 'domcontentloaded' })
  await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {})
  await expectNotRedirectedToLogin(page)
  await expect(page.locator('body')).not.toContainText('404')
}

async function resolveOperationAuditPaths(page) {
  const menuPaths = await page.evaluate(() => {
    const raw = localStorage.getItem('menuList')
    if (!raw) return []

    let menuTree = []
    try {
      menuTree = JSON.parse(raw) || []
    } catch (error) {
      return []
    }

    const preciseMatches = []
    const auditLikeMatches = []

    const walk = (nodes) => {
      if (!Array.isArray(nodes)) return
      nodes.forEach((node) => {
        const name = String(node?.name || '').toLowerCase()
        const component = String(node?.component || '').toLowerCase()
        const path = typeof node?.path === 'string' ? node.path : ''

        const nameMatched = name.includes('操作审计') || name.includes('操作日志')
        const compMatched = component.includes('audit/operationlog') || component.includes('operationlog')
        const auditLike = path.includes('audit')

        if (path && path !== '/audit') {
          if (nameMatched || compMatched) {
            preciseMatches.push(path)
          } else if (auditLike) {
            auditLikeMatches.push(path)
          }
        }

        if (Array.isArray(node?.children) && node.children.length > 0) {
          walk(node.children)
        }
      })
    }

    walk(menuTree)
    return [...preciseMatches, ...auditLikeMatches]
  })

  return [
    ...menuPaths,
    '/sys/audit/operationLog',
    '/sys/audit/operation-log',
    '/sys/audit/operation/log',
    '/sys/audit/operation/logs',
    '/sys/audit/operate-log',
    '/sys/audit/operation',
  ].filter((item, index, arr) => item && arr.indexOf(item) === index)
}

async function openOperationAuditPage(page) {
  const candidates = await resolveOperationAuditPaths(page)

  for (const path of candidates) {
    try {
      await openAndWait(page, path)
      await expect(page.getByPlaceholder('用户名 / 路径 / IP / 说明')).toBeVisible({ timeout: 2500 })
      return path
    } catch (error) {
      // Try next candidate path.
    }
  }

  throw new Error(`Unable to open operation audit page from candidates: ${candidates.join(', ')}`)
}

test.describe('system operation audit', () => {
  test('operation audit search and detail modal', async ({ page }) => {
    await openAndWait(page, '/index')

    await openOperationAuditPage(page)

    const searchInput = page.getByPlaceholder('用户名 / 路径 / IP / 说明')
    await expect(searchInput).toBeVisible()
    await searchInput.fill('admin')
    await searchInput.press('Enter')
    await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {})

    await expect(page.locator('.audit-page .ant-table').first()).toBeVisible()

    const detailBtn = page.getByRole('button', { name: '查看详情' }).first()
    if ((await detailBtn.count()) > 0 && (await detailBtn.isVisible())) {
      await detailBtn.click()
      await expect(page.getByText('操作日志详情').first()).toBeVisible()
      await page.keyboard.press('Escape')
    }
  })
})
