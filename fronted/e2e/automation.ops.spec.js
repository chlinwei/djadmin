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

async function resolveMenuPath(page, { nameKeywords = [], componentKeywords = [] }, fallbackPath) {
  const menuPath = await page.evaluate(({ nameKeywords, componentKeywords }) => {
    const raw = localStorage.getItem('menuList')
    if (!raw) return ''

    let menuTree = []
    try {
      menuTree = JSON.parse(raw) || []
    } catch (error) {
      return ''
    }

    const lowerNameKeywords = nameKeywords.map((item) => String(item).toLowerCase())
    const lowerComponentKeywords = componentKeywords.map((item) => String(item).toLowerCase())

    let found = ''
    const walk = (nodes) => {
      if (!Array.isArray(nodes) || found) return
      nodes.forEach((node) => {
        if (found) return

        const name = String(node?.name || '').toLowerCase()
        const component = String(node?.component || '').toLowerCase()
        const path = typeof node?.path === 'string' ? node.path : ''

        const byName = lowerNameKeywords.some((keyword) => name.includes(keyword))
        const byComponent = lowerComponentKeywords.some((keyword) => component.includes(keyword))

        if (path && (byName || byComponent)) {
          found = path
          return
        }

        if (Array.isArray(node?.children) && node.children.length > 0) {
          walk(node.children)
        }
      })
    }

    walk(menuTree)
    return found
  }, { nameKeywords, componentKeywords })

  return menuPath || fallbackPath
}

test.describe('automation ops pages', () => {
  test('automation task page search', async ({ page }) => {
    await openAndWait(page, '/index')

    const taskPath = await resolveMenuPath(
      page,
      { nameKeywords: ['自动化任务', '任务'], componentKeywords: ['sys/automation/index'] },
      '/sys/automation'
    )
    await openAndWait(page, taskPath)

    const searchInput = page.getByPlaceholder('搜索任务名称 / 任务编码')
    await expect(searchInput).toBeVisible()
    await searchInput.fill('demo-task')
    await searchInput.press('Enter')
    await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {})

    await expect(page.getByText('任务列表').first()).toBeVisible()
    await expect(page.locator('.automation-page .ant-table').first()).toBeVisible()
  })

  test('automation logs page search', async ({ page }) => {
    await openAndWait(page, '/index')

    const logsPath = await resolveMenuPath(
      page,
      { nameKeywords: ['任务运行记录'], componentKeywords: ['sys/automation/logs'] },
      '/sys/automation/logs'
    )
    await openAndWait(page, logsPath)

    await expect(page.locator('.automation-logs-page .jobs-card .ant-card-head-title')).toContainText('任务运行记录列表')
    await expect(page.locator('.automation-logs-page .ant-table').first()).toBeVisible()

    const visibleSearchSelectors = [
      'input[placeholder="按执行记录ID搜索"]:visible',
      'input[placeholder="搜索任务ID/发起人"]:visible',
      'input[placeholder="按统一日志过滤"]:visible',
    ]

    for (const selector of visibleSearchSelectors) {
      const input = page.locator(selector).first()
      if ((await input.count()) > 0) {
        await input.fill('1')
        await input.press('Enter')
        await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {})
        break
      }
    }
  })

  test('automation playbook page search', async ({ page }) => {
    await openAndWait(page, '/index')

    const playbookPath = await resolveMenuPath(
      page,
      { nameKeywords: ['playbook模板', '模板'], componentKeywords: ['sys/playbooktemplate'] },
      '/sys/automation/playbooks'
    )
    await openAndWait(page, playbookPath)

    const searchInput = page.getByPlaceholder('搜索模板名称')
    await expect(searchInput).toBeVisible()
    await searchInput.fill('base')
    await searchInput.press('Enter')
    await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {})

    await expect(page.getByText('Playbook 模板').first()).toBeVisible()
  })

  test('automation inventory page search', async ({ page }) => {
    await openAndWait(page, '/index')

    const inventoryPath = await resolveMenuPath(
      page,
      { nameKeywords: ['inventory管理', 'inventory'], componentKeywords: ['sys/automation/inventory'] },
      '/sys/automation/inventory'
    )
    await openAndWait(page, inventoryPath)

    const searchInput = page.getByPlaceholder('搜索 Inventory 名称')
    await expect(searchInput).toBeVisible()
    await searchInput.fill('prod')
    await searchInput.press('Enter')
    await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {})

    await expect(page.getByText('Inventory列表').first()).toBeVisible()
  })

  test('automation workflow page search', async ({ page }) => {
    await openAndWait(page, '/index')

    const workflowPath = await resolveMenuPath(
      page,
      { nameKeywords: ['workflow编排', 'workflow'], componentKeywords: ['sys/automation/workflow'] },
      '/sys/automation/workflow'
    )
    await openAndWait(page, workflowPath)

    const searchInput = page.getByPlaceholder('搜索 Workflow 名称')
    await expect(searchInput).toBeVisible()
    await searchInput.fill('daily')
    await searchInput.press('Enter')
    await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {})

    await expect(page.getByText('Workflow 列表').first()).toBeVisible()
  })

  test('automation workflow page edit button clickable', async ({ page }) => {
    await openAndWait(page, '/index')

    const workflowPath = await resolveMenuPath(
      page,
      { nameKeywords: ['workflow编排', 'workflow'], componentKeywords: ['sys/automation/workflow'] },
      '/sys/automation/workflow'
    )
    await openAndWait(page, workflowPath)

    await expect(page.getByText('Workflow 列表').first()).toBeVisible()

    const editBtn = page
      .locator('.workflow-page .ant-table tbody button:has(svg[data-icon="pen-to-square"]):not([disabled])')
      .first()

    const editableCount = await editBtn.count()
    test.skip(editableCount === 0, '当前数据或权限下没有可编辑的 Workflow')

    await editBtn.click()
    await page.waitForURL('**/sys/automation/workflow/editor**', { timeout: 10000 })
    await expect(page).toHaveURL(/\/sys\/automation\/workflow\/editor/)
  })

  test('automation workflow page launch modal', async ({ page }) => {
    await openAndWait(page, '/index')

    const workflowPath = await resolveMenuPath(
      page,
      { nameKeywords: ['workflow编排', 'workflow'], componentKeywords: ['sys/automation/workflow'] },
      '/sys/automation/workflow'
    )
    await openAndWait(page, workflowPath)

    await expect(page.getByText('Workflow 列表').first()).toBeVisible()

    const launchBtn = page
      .locator('.workflow-page .ant-table tbody button:has(svg[data-icon="play"]):not([disabled])')
      .first()

    const launchableCount = await launchBtn.count()
    test.skip(launchableCount === 0, '当前数据或权限下没有可启动的 Workflow')

    await launchBtn.click()
    await expect(page.getByText('确认启动 Workflow').first()).toBeVisible()

    await page.keyboard.press('Escape')
  })

  test('automation workflow launch tooltip should not block click', async ({ page }) => {
    await openAndWait(page, '/index')

    const workflowPath = await resolveMenuPath(
      page,
      { nameKeywords: ['workflow编排', 'workflow'], componentKeywords: ['sys/automation/workflow'] },
      '/sys/automation/workflow'
    )
    await openAndWait(page, workflowPath)

    await expect(page.getByText('Workflow 列表').first()).toBeVisible()

    const launchBtn = page
      .locator('.workflow-page .ant-table tbody button:has(svg[data-icon="play"]):not([disabled])')
      .first()

    const launchableCount = await launchBtn.count()
    test.skip(launchableCount === 0, '当前数据或权限下没有可启动的 Workflow')

    await launchBtn.hover()
    const runTooltip = page.locator('.ant-tooltip:visible', { hasText: '运行' }).first()
    await expect(runTooltip).toBeVisible()

    // Critical regression check: clicking while tooltip is visible must still open launch modal.
    await launchBtn.click()
    await expect(page.getByText('确认启动 Workflow').first()).toBeVisible()

    await page.keyboard.press('Escape')
  })

  test('automation workflow launch tooltip should appear above button', async ({ page }) => {
    await openAndWait(page, '/index')

    const workflowPath = await resolveMenuPath(
      page,
      { nameKeywords: ['workflow编排', 'workflow'], componentKeywords: ['sys/automation/workflow'] },
      '/sys/automation/workflow'
    )
    await openAndWait(page, workflowPath)

    await expect(page.getByText('Workflow 列表').first()).toBeVisible()

    const launchBtn = page
      .locator('.workflow-page .ant-table tbody button:has(svg[data-icon="play"]):not([disabled])')
      .first()

    const launchableCount = await launchBtn.count()
    test.skip(launchableCount === 0, '当前数据或权限下没有可启动的 Workflow')

    await launchBtn.hover()

    const tooltip = page.locator('.ant-tooltip:visible', { hasText: '运行' }).first()
    await expect(tooltip).toBeVisible()
    await expect(tooltip).toHaveClass(/ant-tooltip-placement-top/)

    const buttonBox = await launchBtn.boundingBox()
    const tooltipBox = await tooltip.boundingBox()
    expect(buttonBox).not.toBeNull()
    expect(tooltipBox).not.toBeNull()

    // Tooltip should be above the button (allow a 1px tolerance for sub-pixel layout).
    expect(tooltipBox.y + tooltipBox.height).toBeLessThanOrEqual(buttonBox.y + 1)

    // Tooltip should keep a visible vertical gap from the button, not overlap it.
    const verticalGap = buttonBox.y - (tooltipBox.y + tooltipBox.height)
    expect(verticalGap).toBeGreaterThanOrEqual(2)
  })
})
