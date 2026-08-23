import { expect, test } from '@playwright/test'

test('application configuration remains available as a child route', async ({ page }) => {
  await page.goto('/assets/applications/index')
  await expect(page.getByRole('tab', { name: '业务系统' })).toBeVisible()
  await expect(page.getByRole('tab', { name: '应用定义' })).toBeVisible()
})

test('service tree shows node summaries, aggregate metrics, and direct children', async ({ page }) => {
  await page.route('**/assets/business-systems/**', async (route) => {
    const isDetail = new URL(route.request().url()).pathname.endsWith('/assets/business-systems/7/')
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 200, msg: 'success', data: isDetail ? {
        id: 7, name: '订单系统', code: 'order-system', owner: '订单团队', enabled: true,
      } : {
        results: [{ id: 7, name: '订单系统', code: 'order-system', enabled: true }],
        count: 1, pageNumber: 1, pageSize: 30, totalPages: 1, next: null, previous: null,
      } }),
    })
  })
  await page.route('**/assets/application-deployments/**', async (route) => {
    const pathname = new URL(route.request().url()).pathname
    const isDetail = pathname.endsWith('/assets/application-deployments/11/')
    const isHistory = pathname.endsWith('/assets/application-deployments/11/baseline-history/')
    const deployment = {
      id: 11,
      application_service: 21,
      service_name: '订单服务',
      business_system: 7,
      business_system_name: '订单系统',
      application_name: 'Order API',
      version: '1.0',
      instance_name: 'order-test-1',
      member_role: 'standalone',
      environment: 'testing',
      host_name: 'order-node-1',
      host_ip: '10.0.0.31',
      runtime_status: 'running',
      health_status: 'healthy',
      baseline_pass_rate: 100,
      ports: [{ name: 'HTTP', protocol: 'tcp', port: 8080 }],
      enabled: true,
    }
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 200, msg: 'success', data: isHistory ? [{
        id: 41, status: 'completed', passed: true, passed_count: 3, total_count: 3, requested_username: 'admin',
      }] : isDetail ? deployment : {
        results: [deployment],
        count: 1, pageNumber: 1, pageSize: 30, totalPages: 1, next: null, previous: null,
      } }),
    })
  })
  await page.route('**/assets/application-services/**', async (route) => {
    const isDetail = new URL(route.request().url()).pathname.endsWith('/assets/application-services/21/')
    const service = {
      id: 21,
      business_system: 7,
      business_system_name: '订单系统',
      name: '订单服务',
      environment: 'testing',
      topology_type: 'standalone',
      application_name: 'Order API',
      deployment_count: 1,
      availability_mode: 'none',
      access_type: 'direct',
      enabled: true,
    }
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 200, msg: 'success', data: isDetail ? service : {
        results: [service],
        count: 1, pageNumber: 1, pageSize: 30, totalPages: 1, next: null, previous: null,
      } }),
    })
  })
  await page.goto('/assets/service-tree/index')
  await expect(page.locator('.service-tree-title')).toHaveText('服务树')
  await expect(page.locator('.service-tree')).toContainText('全部业务')
  await expect(page.locator('.node-content')).toContainText('业务系统1')
  await expect(page.locator('.node-content')).toContainText('逻辑服务1')

  const businessSystem = page.locator('.service-tree .ant-tree-node-content-wrapper')
    .filter({ hasText: '订单系统' })
    .first()
  await businessSystem.click()
  await expect(page.locator('.node-content')).toContainText('订单团队')
  await expect(page.locator('.node-content')).toContainText('测试环境')

  const testingEnvironment = page.locator('.service-tree .ant-tree-node-content-wrapper')
    .filter({ hasText: '测试环境' })
    .first()
  await expect(testingEnvironment).toBeVisible()
  const serviceResponse = page.waitForResponse((response) => {
    const url = new URL(response.url())
    return url.pathname.endsWith('/assets/application-services/')
      && url.searchParams.get('environment') === 'testing'
      && url.searchParams.get('business_system') === '7'
  })
  await testingEnvironment.click()
  await serviceResponse
  await expect(page.locator('.node-content')).toContainText('部署实例1')
  await expect(page.locator('.node-content')).toContainText('订单服务')

  const logicalService = page.locator('.service-tree .ant-tree-node-content-wrapper')
    .filter({ hasText: '订单服务' })
    .first()
  const deploymentResponse = page.waitForResponse((response) => {
    const url = new URL(response.url())
    return url.pathname.endsWith('/assets/application-deployments/')
      && url.searchParams.get('application_service') === '21'
  })
  await logicalService.click()
  await deploymentResponse
  await expect(page.locator('.node-content')).toContainText('实例节点地址')
  await expect(page.locator('.node-content')).toContainText('order-test-1')

  const deployment = page.locator('.service-tree .ant-tree-node-content-wrapper')
    .filter({ hasText: 'order-test-1' })
    .first()
  await deployment.click()
  await expect(page.locator('.node-content')).toContainText('10.0.0.31')
  await expect(page.locator('.node-content')).toContainText('TCP 8080')
  await expect(page.locator('.node-content')).toContainText('最近基线检查')
})
