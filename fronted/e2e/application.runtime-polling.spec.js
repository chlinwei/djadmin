import { expect, test } from '@playwright/test'

test('deployment refresh sends status requests and polling repeats them', async ({ page }) => {
  test.setTimeout(40000)
  const statusRequestTimes = []

  page.on('request', (request) => {
    if (!/\/application-deployments\/\d+\/control\/$/.test(request.url())) return
    if (request.postDataJSON()?.action === 'status') statusRequestTimes.push(Date.now())
  })

  await page.goto('/assets/applications/index')
  await page.getByRole('tab', { name: '部署实例' }).click()
  await expect(page.getByRole('button', { name: /登记实例/ })).toBeVisible()

  statusRequestTimes.length = 0
  await page.getByRole('button', { name: /刷新/ }).click()
  await expect.poll(() => statusRequestTimes.length, { timeout: 5000 }).toBeGreaterThan(0)

  const manualRefreshTime = statusRequestTimes[0]
  await expect.poll(
    () => Math.max(...statusRequestTimes) - manualRefreshTime,
    { timeout: 25000, intervals: [500] },
  ).toBeGreaterThanOrEqual(9000)
})