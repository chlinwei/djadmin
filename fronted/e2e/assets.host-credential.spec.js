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

async function collectMenuPaths(page, matchFn) {
  return await page.evaluate((matcherSource) => {
    const raw = localStorage.getItem('menuList')
    if (!raw) return []

    let menuTree = []
    try {
      menuTree = JSON.parse(raw) || []
    } catch (error) {
      return []
    }

    const matcher = new Function('name', 'component', 'path', matcherSource)
    const paths = []

    const walk = (nodes) => {
      if (!Array.isArray(nodes)) return
      nodes.forEach((node) => {
        const name = String(node?.name || '').toLowerCase()
        const component = String(node?.component || '').toLowerCase()
        const path = typeof node?.path === 'string' ? node.path : ''

        if (path && matcher(name, component, path)) {
          paths.push(path)
        }

        if (Array.isArray(node?.children) && node.children.length > 0) {
          walk(node.children)
        }
      })
    }

    walk(menuTree)
    return paths
  }, matchFn)
}

function uniqueItems(items) {
  return items.filter((item, index, arr) => item && arr.indexOf(item) === index)
}

async function openByCandidates(page, candidates, detector) {
  for (const path of candidates) {
    try {
      await openAndWait(page, path)
      const ok = await detector()
      if (ok) {
        return path
      }
    } catch (error) {
      // Try next candidate.
    }
  }
  throw new Error(`No matching page found in candidates: ${candidates.join(', ')}`)
}

test.describe('assets host and credential', () => {
  test('host management page search and table render', async ({ page }) => {
    await openAndWait(page, '/index')

    const matchedMenuPaths = await collectMenuPaths(
      page,
      "return (name.includes('主机') || name.includes('host')) && (component.includes('assets/host') || path.includes('/assets/host'));"
    )

    const candidatePaths = uniqueItems([
      ...matchedMenuPaths,
      '/assets/hosts',
      '/assets/host',
      '/assets/hosts/index',
      '/assets/host/index',
    ])

    await openByCandidates(page, candidatePaths, async () => {
      const hostSearch = page.getByPlaceholder('主机名 / IP / 备注')
      return (await hostSearch.count()) > 0 && (await hostSearch.first().isVisible())
    })

    const hostSearch = page.getByPlaceholder('主机名 / IP / 备注')
    await hostSearch.first().fill('127.0.0.1')
    await hostSearch.first().press('Enter')
    await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {})

    await expect(page.locator('.host-page .host-card .ant-table').first()).toBeVisible()
    await expect(page.locator('th').filter({ hasText: 'IP 地址' }).first()).toBeVisible()
  })

  test('credential management page search and table render', async ({ page }) => {
    await openAndWait(page, '/index')

    const matchedMenuPaths = await collectMenuPaths(
      page,
      "return (name.includes('凭证') || name.includes('credential')) && (component.includes('assets/credential') || path.includes('/assets/credential'));"
    )

    const candidatePaths = uniqueItems([
      ...matchedMenuPaths,
      '/assets/credential',
      '/assets/credentials',
      '/assets/credential/index',
      '/assets/credentials/index',
    ])

    await openByCandidates(page, candidatePaths, async () => {
      const hasCredentialColumn = await page.locator('th').filter({ hasText: '凭证名称' }).count()
      return hasCredentialColumn > 0
    })

    const searchInput = page.getByPlaceholder('全局搜索')
    await expect(searchInput.first()).toBeVisible()
    await searchInput.first().fill('admin')
    await searchInput.first().press('Enter')
    await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {})

    await expect(page.locator('th').filter({ hasText: '凭证名称' }).first()).toBeVisible()
    await expect(page.locator('.ant-table').first()).toBeVisible()
  })
})
