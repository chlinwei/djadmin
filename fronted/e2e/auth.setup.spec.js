import { test, expect } from '@playwright/test'

test('login and save storage state', async ({ page, context }) => {
  const username = 'admin'
  const password = 'admin'

  await page.goto('/login')

  await page.locator('#basic_username').fill(username)
  await page.locator('#basic_password').fill(password)
  await page.getByRole('button', { name: '登录' }).click()

  await page.waitForURL('**/index**', { timeout: 15000 })
  await expect(page).not.toHaveURL(/\/login/)

  await context.storageState({ path: 'playwright/.auth/user.json' })
})
