import { buildAutomationInventoryRoute, buildAutomationTemplateRoute, getAutomationTemplateName } from '../utils/navigation'

describe('automation navigation helpers', () => {
  it('removes only the API presentation prefix for display and template search', () => {
    const record = { template_name: '[Playbook] [Playbook] business-name', template_content_format: 'shell' }
    expect(getAutomationTemplateName(record)).toBe('[Playbook] business-name')
    expect(buildAutomationTemplateRoute(record).query.search).toBe('[Playbook] business-name')
    expect(record.template_name).toBe('[Playbook] [Playbook] business-name')
  })

  it('builds template route with playbook type and search when template name exists', () => {
    const route = buildAutomationTemplateRoute({
      template_name: 'Deploy Nginx',
    })

    expect(route).toEqual({
      path: '/sys/automation/templates',
      query: {
        search: 'Deploy Nginx',
      },
    })
  })

  it('builds template route with playbook type when template name is blank', () => {
    const route = buildAutomationTemplateRoute({
      template_name: '   ',
    })

    expect(route).toEqual({
      path: '/sys/automation/templates',
      query: {},
    })
  })

  it('builds inventory route with search and inventory id', () => {
    const route = buildAutomationInventoryRoute({
      inventory: 7,
      inventory_name: '生产环境 Inventory',
    })

    expect(route).toEqual({
      path: '/sys/automation/inventory',
      query: {
        search: '生产环境 Inventory',
        inventory_name: '生产环境 Inventory',
        inventory_id: '7',
      },
    })
  })

  it('builds empty query when inventory name is blank and id is invalid', () => {
    const route = buildAutomationInventoryRoute({
      inventory: 'abc',
      inventory_name: '   ',
    })

    expect(route).toEqual({
      path: '/sys/automation/inventory',
      query: {},
    })
  })
})
