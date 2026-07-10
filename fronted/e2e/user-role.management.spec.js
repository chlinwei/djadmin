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

async function resolveMenuPath(page, keywords, fallbackPath) {
  const path = await page.evaluate((keywordList) => {
    const raw = localStorage.getItem('menuList')
    if (!raw) return ''

    let menuTree = []
    try {
      menuTree = JSON.parse(raw) || []
    } catch (error) {
      return ''
    }

    const lowerKeywords = keywordList.map((item) => String(item).toLowerCase())
    let result = ''

    const walk = (nodes) => {
      if (!Array.isArray(nodes) || result) return
      nodes.forEach((node) => {
        if (result) return

        const name = String(node?.name || '').toLowerCase()
        const path = typeof node?.path === 'string' ? node.path : ''
        if (path && lowerKeywords.some((keyword) => name.includes(keyword))) {
          result = path
          return
        }

        if (Array.isArray(node?.children) && node.children.length > 0) {
          walk(node.children)
        }
      })
    }

    walk(menuTree)
    return result
  }, keywords)

  return path || fallbackPath
}

test.describe('user and role management', () => {
  test('user management page search and table load', async ({ page }) => {
    await openAndWait(page, '/index')

    const userPath = await resolveMenuPath(page, ['用户', 'user'], '/sys/user')
    await openAndWait(page, userPath)

    const searchInput = page.getByPlaceholder('用户名/邮箱/手机号/备注')
    await expect(searchInput).toBeVisible()
    await searchInput.fill('admin')
    await searchInput.press('Enter')
    await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {})

    await expect(page.locator('.ant-table').first()).toBeVisible()
    await expect(page.locator('th').filter({ hasText: '用户名' }).first()).toBeVisible()
    await expect(page.locator('th').filter({ hasText: '角色' }).first()).toBeVisible()
  })

  test('role management page search and table load', async ({ page }) => {
    await openAndWait(page, '/index')

    const rolePath = await resolveMenuPath(page, ['角色', 'role'], '/sys/role')
    await openAndWait(page, rolePath)

    const searchInput = page.getByPlaceholder('角色名/权限字符/备注')
    await expect(searchInput).toBeVisible()
    await searchInput.fill('admin')
    await searchInput.press('Enter')
    await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {})

    await expect(page.locator('.ant-table').first()).toBeVisible()
    await expect(page.locator('th').filter({ hasText: '角色名' }).first()).toBeVisible()
    await expect(page.locator('th').filter({ hasText: '权限字符' }).first()).toBeVisible()
  })
})
