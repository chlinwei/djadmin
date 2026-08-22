import { test, expect } from '@playwright/test'

test('application workspace edits schema baseline checks without Vue update errors', async ({ page }) => {
  test.setTimeout(60000)
  const pageErrors = []
  const runId = Date.now()
  const applicationName = `模板绑定回归应用-${runId}`
  const applicationCode = `template-binding-e2e-${runId}`
  page.on('pageerror', (error) => pageErrors.push(error.message))

  await page.goto('/assets/applications/index')
  await expect(page.getByRole('tab', { name: '应用定义' })).toBeVisible()

  for (let iteration = 0; iteration < 3; iteration += 1) {
    await page.getByRole('tab', { name: '部署模板' }).click()
    await expect(page.getByRole('button', { name: /新增模板/ })).toBeVisible()
    await page.getByRole('tab', { name: '部署实例' }).click()
    await expect(page.getByRole('button', { name: /登记实例/ })).toBeVisible()
    await page.getByRole('tab', { name: '应用定义' }).click()
  }

  await page.getByRole('tab', { name: '应用定义' }).click()
  await page.getByRole('button', { name: /新增应用/ }).click()
  await page.getByRole('textbox', { name: /应用名称/ }).fill(applicationName)
  await page.getByRole('textbox', { name: /应用编码/ }).fill(applicationCode)
  await page.locator('.baseline-type-select .ant-select-selector').click()
  await page.getByText('JSON', { exact: true }).last().click()
  await expect(page.locator('.baseline-check-item')).toHaveCount(0)
  await page.getByRole('button', { name: /新增检查项/ }).click()
  await expect(page.locator('.baseline-check-item')).toHaveCount(1)
  await expect(page.locator('.baseline-check-item').first()).toContainText('JSON · JSON Schema')
  await expect(page.locator('textarea.schema-editor')).toHaveValue(/draft\/2020-12\/schema/)
  await page.locator('.ant-modal-content:visible .ant-modal-footer .ant-btn-primary').click()
  await expect(page.getByText('请完整填写检查项 1 的名称和文件路径', { exact: true })).toBeVisible()
  await page.locator('.baseline-check-item .delBtn').click()
  await page.getByRole('button', { name: '确认删除' }).click()
  await expect(page.locator('.ant-modal-confirm-centered')).not.toBeVisible()
  await page.locator('.baseline-type-select .ant-select-selector').click()
  await page.getByText('普通文本', { exact: true }).last().click()
  await page.getByRole('button', { name: /新增检查项/ }).click()
  await expect(page.locator('.baseline-check-item').first()).toContainText('普通文本 · Regexp')
  await expect(page.getByText('规则版本', { exact: true })).toBeVisible()
  await expect(page.locator('textarea.schema-editor')).toHaveValue(/"expect": "present"/)
  await expect(page.locator('textarea.schema-editor')).toHaveValue(/\(\?m\)\^debug/)
  await page.locator('.baseline-check-item .delBtn').click()
  await page.getByRole('button', { name: '确认删除' }).click()
  await expect(page.locator('.ant-modal-confirm-centered')).not.toBeVisible()
  for (const documentType of ['INI', 'TOML', 'Properties']) {
    await page.locator('.baseline-type-select .ant-select-selector').click()
    await page.getByText(documentType, { exact: true }).last().click()
    await page.getByRole('button', { name: /新增检查项/ }).click()
    await expect(page.locator('.baseline-check-item').first()).toContainText(`${documentType} · JSON Schema`)
    await expect(page.locator('textarea.schema-editor')).toHaveValue(/draft\/2020-12\/schema/)
    await page.locator('.baseline-check-item .delBtn').click()
    await page.getByRole('button', { name: '确认删除' }).click()
    await expect(page.locator('.ant-modal-confirm-centered')).not.toBeVisible()
  }
  for (let iteration = 0; iteration < 3; iteration += 1) {
    await page.getByRole('button', { name: /新增检查项/ }).click()
    await expect(page.getByText('检查项 1', { exact: true })).toBeVisible()
    await expect(page.getByText('校验规则', { exact: true })).toBeVisible()
    await expect(page.getByPlaceholder('例如 ${APP_HOME}/conf/application.xml')).toBeVisible()
    await page.locator('.baseline-check-item .delBtn').click()
    await page.getByRole('button', { name: '确认删除' }).click()
    await expect(page.locator('.ant-modal-confirm-centered')).not.toBeVisible()
    await expect(page.getByText('检查项 1', { exact: true })).not.toBeVisible()
    expect(pageErrors).toEqual([])
  }
  const cancelVisibleDialog = () => page.locator('.ant-modal-wrap:not(.ant-modal-confirm-centered) .ant-modal-close').click()
  await cancelVisibleDialog()
  expect(pageErrors, '首次关闭新增弹窗时发生错误').toEqual([])
  await page.getByRole('button', { name: /新增应用/ }).click()
  await expect(page.getByRole('button', { name: /新增检查项/ })).toBeVisible()
  await page.getByRole('textbox', { name: /应用名称/ }).fill(applicationName)
  await page.getByRole('textbox', { name: /应用编码/ }).fill(applicationCode)
  await page.getByRole('button', { name: /新增检查项/ }).click()
  await page.getByPlaceholder('例如 XML 属性合规检查').fill('XML 安全基线')
  await page.getByPlaceholder('例如 ${APP_HOME}/conf/application.xml').fill('${APP_HOME}/conf/application.xml')
  await expect(page.locator('textarea.schema-editor')).toHaveValue(/Schematron|application-baseline/)
  await page.locator('.ant-modal-content:visible .ant-modal-footer .ant-btn-primary').click()
  await expect(page.locator('.ant-modal-content:visible')).not.toBeVisible()
  await expect(page.getByRole('row').filter({ hasText: applicationName })).toBeVisible()
  await page.getByRole('tab', { name: '部署模板' }).click()
  expect(pageErrors, '切换到部署模板时发生错误').toEqual([])
  await page.getByRole('tab', { name: '应用定义' }).click()
  expect(pageErrors, '切回应用定义时发生错误').toEqual([])

  for (let iteration = 0; iteration < 3; iteration += 1) {
    await page.getByRole('row').filter({ hasText: applicationName }).locator('button').first().click()
    await expect(page.locator('.ant-modal-content:visible')).toBeVisible()
    expect(pageErrors, `编辑弹窗第 ${iteration + 1} 次打开时发生错误`).toEqual([])
    await expect(page.getByRole('button', { name: /新增检查项/ })).toBeVisible()
    await expect(page.locator('.schema-type-select .ant-select-selection-item')).toBeVisible()
    expect(pageErrors, `编辑弹窗第 ${iteration + 1} 次加载详情时发生错误`).toEqual([])
    if (iteration === 0) {
      const checkCards = page.locator('.baseline-check-item')
      const previousCount = await checkCards.count()
      await page.getByRole('button', { name: /新增检查项/ }).click()
      await expect(checkCards).toHaveCount(previousCount + 1)
      await expect(checkCards.first()).toContainText('检查项 1')
    }
    await cancelVisibleDialog()
    expect(pageErrors, `编辑弹窗第 ${iteration + 1} 次关闭时发生错误`).toEqual([])
  }

  expect(pageErrors).toEqual([])
})