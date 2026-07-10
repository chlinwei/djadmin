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

async function fillSearchAndSubmit(page, placeholder, keyword) {
  const search = page.getByPlaceholder(placeholder)
  if ((await search.count()) === 0) return false
  if (!(await search.first().isVisible())) return false

  await search.first().fill(keyword)
  await search.first().press('Enter')
  await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {})
  // Defocus search input to avoid overlay/input intercepting subsequent clicks.
  await page.mouse.click(8, 8)
  return true
}

test.describe('menu light actions', () => {
  test('automation task page search and create modal', async ({ page }) => {
    await openAndWait(page, '/sys/automation')
    await expect(page.getByText('任务列表').first()).toBeVisible()

    const searched = await fillSearchAndSubmit(page, '搜索任务名称 / 任务编码', 'demo-task')
    expect(searched).toBeTruthy()
    await expect(page.locator('.automation-page .right-actions .ant-btn').first()).toBeVisible()
  })

  test('playbook page search and create modal', async ({ page }) => {
    await openAndWait(page, '/sys/automation/playbooks')
    await expect(page.getByText('Playbook 模板').first()).toBeVisible()

    const searched = await fillSearchAndSubmit(page, '搜索模板名称', 'base')
    expect(searched).toBeTruthy()
    await expect(page.locator('.playbook-template-page .ant-table, .playbook-template-page .ant-card').first()).toBeVisible()
  })

  test('inventory page search and create modal', async ({ page }) => {
    await openAndWait(page, '/sys/automation/inventory')
    await expect(page.getByText('Inventory列表').first()).toBeVisible()

    const searched = await fillSearchAndSubmit(page, '搜索 Inventory 名称', 'prod')
    expect(searched).toBeTruthy()
    await expect(page.locator('.inventory-page .right-actions .ant-btn').first()).toBeVisible()
  })

  test('scheduler page search and refresh', async ({ page }) => {
    await openAndWait(page, '/sys/scheduler')
    await expect(page.locator('.scheduler-page .ant-table').first()).toBeVisible()

    const searched = await fillSearchAndSubmit(page, '搜索任务名称 / 任务编码', 'health-check')
    expect(searched).toBeTruthy()
    await expect(page.locator('.scheduler-page .right-actions .ant-btn').first()).toBeVisible()
  })
})
