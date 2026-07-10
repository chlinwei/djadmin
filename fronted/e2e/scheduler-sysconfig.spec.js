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

test.describe('scheduler and sysconfig', () => {
  test('scheduler page search and table render', async ({ page }) => {
    await openAndWait(page, '/index')

    const schedulerPath = await resolveMenuPath(page, ['定时任务', 'scheduler'], '/sys/scheduler')
    await openAndWait(page, schedulerPath)

    const searchInput = page.getByPlaceholder('搜索任务名称 / 任务编码')
    await expect(searchInput).toBeVisible()
    await searchInput.fill('health-check')
    await searchInput.press('Enter')
    await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {})

    await expect(page.locator('.scheduler-page .ant-table').first()).toBeVisible()
    await expect(page.locator('.scheduler-page .ant-card').first()).toBeVisible()
  })

  test('sysconfig page search, filter and edit modal open', async ({ page }) => {
    await openAndWait(page, '/index')

    const sysconfigPath = await resolveMenuPath(page, ['参数', 'sysconfig', '配置'], '/sys/sysconfig')
    await openAndWait(page, sysconfigPath)

    const searchInput = page.getByPlaceholder('搜索参数名称 / 参数键')
    await expect(searchInput).toBeVisible()
    await searchInput.fill('timezone')
    await searchInput.press('Enter')
    await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {})

    await expect(page.locator('.sysconfig-page .ant-table').first()).toBeVisible()

    const changedOnly = page.locator('.sysconfig-page .ant-radio-button-wrapper').filter({ hasText: '仅已修改' })
    if ((await changedOnly.count()) > 0 && (await changedOnly.first().isVisible())) {
      await changedOnly.first().click()
      await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {})
    }

    const editBtn = page.getByRole('button', { name: '编辑' }).first()
    if ((await editBtn.count()) > 0) {
      await editBtn.click()
      await expect(page.getByText('修改参数').first()).toBeVisible()
      await page.keyboard.press('Escape')
    }
  })
})
