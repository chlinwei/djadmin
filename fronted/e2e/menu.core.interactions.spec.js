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

test.describe('core menu interactions', () => {
  test('core pages basic interactions', async ({ page }) => {
    test.setTimeout(5 * 60 * 1000)

    await openAndWait(page, '/index')
    await expect(page.locator('body')).toContainText('首页')

    await openAndWait(page, '/sys/userCenter')
    await expect(page.getByText('个人信息').first()).toBeVisible()
    await page.getByRole('tab', { name: '修改密码' }).click()
    await expect(page.getByLabel('旧密码').first()).toBeVisible()

    await openAndWait(page, '/sys/scheduler')
    const schedulerSearch = page.getByPlaceholder('搜索任务名称 / 任务编码')
    await expect(schedulerSearch).toBeVisible()
    await schedulerSearch.fill('health-check')
    await schedulerSearch.press('Enter')
    await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {})
    await expect(page.locator('.ant-table').first()).toBeVisible()

    await openAndWait(page, '/sys/automation')
    const automationSearch = page.getByPlaceholder('搜索任务名称 / 任务编码')
    await expect(automationSearch).toBeVisible()
    await automationSearch.fill('demo-task')
    await automationSearch.press('Enter')
    await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {})
    await expect(page.getByText('任务列表').first()).toBeVisible()
  })

  test('automation sub pages basic interactions', async ({ page }) => {
    test.setTimeout(5 * 60 * 1000)

    await openAndWait(page, '/sys/automation/workflow')
    const workflowSearch = page.getByPlaceholder('搜索 Workflow 名称')
    await expect(workflowSearch).toBeVisible()
    await workflowSearch.fill('daily')
    await workflowSearch.press('Enter')
    await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {})
    await expect(page.getByText('Workflow 列表').first()).toBeVisible()

    await openAndWait(page, '/sys/automation/playbooks')
    const playbookSearch = page.getByPlaceholder('搜索模板名称')
    await expect(playbookSearch).toBeVisible()
    await playbookSearch.fill('base')
    await playbookSearch.press('Enter')
    await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {})
    await expect(page.getByText('Playbook 模板').first()).toBeVisible()

    await openAndWait(page, '/sys/automation/inventory')
    const inventorySearch = page.getByPlaceholder('搜索 Inventory 名称')
    await expect(inventorySearch).toBeVisible()
    await inventorySearch.fill('prod')
    await inventorySearch.press('Enter')
    await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {})
    await expect(page.getByText('Inventory列表').first()).toBeVisible()

    await openAndWait(page, '/sys/automation/logs')
    await expect(page.locator('.automation-logs-page .jobs-card .ant-card-head-title')).toContainText('任务运行记录列表')
  })
})
