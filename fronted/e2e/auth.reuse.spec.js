import { test, expect } from '@playwright/test'

test('reuse saved auth state to access index', async ({ page }) => {
  await page.goto('/index')
  await expect(page).not.toHaveURL(/\/login/)

  const token = await page.evaluate(() => window.localStorage.getItem('token'))
  expect(token).toBeTruthy()
})
