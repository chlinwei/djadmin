import { test, expect } from '@playwright/test'

const STATIC_MENU_PATHS = [
  '/index',
  '/sys/userCenter',
  '/sys/scheduler',
  '/sys/automation',
]

async function expectNotRedirectedToLogin(page) {
  const pathname = await page.evaluate(() => window.location.pathname)
  expect(pathname).not.toBe('/login')
}

function shouldSkipPath(path) {
  if (!path || typeof path !== 'string') return true
  if (!path.startsWith('/')) return true
  if (/^https?:\/\//i.test(path)) return true
  if (path.includes(':') || path.includes('*')) return true
  return false
}

async function assertGenericPageHealth(page) {
  await expect(page.locator('body')).not.toContainText('404')
  await expect(page.locator('body')).not.toContainText('Cannot GET')

  const hasCommonWidget = await page
    .locator('.ant-table, .ant-form, .ant-card, .ant-tabs, .ant-descriptions, .ant-list, .ant-empty, .vue-flow, .xterm')
    .count()

  if (hasCommonWidget > 0) {
    await expect(
      page.locator('.ant-table, .ant-form, .ant-card, .ant-tabs, .ant-descriptions, .ant-list, .ant-empty, .vue-flow, .xterm').first()
    ).toBeVisible()
    return
  }

  // Fallback for pages that render minimal structure.
  const hasDomChildren = await page.evaluate(() => (document.body?.childElementCount || 0) > 0)
  expect(hasDomChildren).toBeTruthy()
}

async function assertCorePath(page, path) {
  if (path === '/index') {
    await expect(page.locator('body')).toContainText('首页')
    return
  }

  if (path === '/sys/userCenter') {
    await expect(page.getByText('个人信息').first()).toBeVisible()
    await expect(page.locator('.ant-form').first()).toBeVisible()
    return
  }

  if (path === '/sys/scheduler') {
    await expect(page.getByPlaceholder('搜索任务名称 / 任务编码')).toBeVisible()
    await expect(page.locator('.ant-table').first()).toBeVisible()
    return
  }

  if (path === '/sys/automation') {
    await expect(page.getByText('任务列表').first()).toBeVisible()
    await expect(page.locator('.ant-table').first()).toBeVisible()
  }
}

test.describe('menu smoke', () => {
  test('visit every available menu page', async ({ page }) => {
    test.setTimeout(5 * 60 * 1000)

    await page.goto('/index')
    await expectNotRedirectedToLogin(page)

    const menuPaths = await page.evaluate(() => {
      const raw = localStorage.getItem('menuList')
      if (!raw) return []

      let menuTree = []
      try {
        menuTree = JSON.parse(raw) || []
      } catch (error) {
        return []
      }

      const paths = []
      const walk = (nodes) => {
        if (!Array.isArray(nodes)) return
        nodes.forEach((item) => {
          const children = Array.isArray(item?.children) ? item.children : []
          const component = String(item?.component || '').toLowerCase()
          const isDirectoryLike =
            children.length > 0 &&
            (!component || component.includes('layout') || component.includes('parentview') || component.includes('parent_view') || component.includes('routeview'))

          if (typeof item?.path === 'string' && !isDirectoryLike) {
            paths.push(item.path)
          }
          if (children.length > 0) {
            walk(children)
          }
        })
      }

      walk(menuTree)
      return paths
    })

    const pathsToVisit = Array.from(new Set([...STATIC_MENU_PATHS, ...menuPaths]))
      .filter((path) => !shouldSkipPath(path))

    expect(pathsToVisit.length).toBeGreaterThan(0)

    for (const path of pathsToVisit) {
      await test.step(`open menu path: ${path}`, async () => {
        await page.goto(path, { waitUntil: 'domcontentloaded' })

        // Some pages keep polling in background; networkidle is best effort only.
        await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {})

        await expectNotRedirectedToLogin(page)
        await assertGenericPageHealth(page)
        await assertCorePath(page, path)
      })
    }
  })
})
