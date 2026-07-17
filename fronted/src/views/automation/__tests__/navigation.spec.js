import { buildAutomationInventoryRoute, buildAutomationTemplateRoute } from '../navigation'

describe('automation navigation helpers', () => {
  it('builds template route with playbook type and search when template name exists', () => {
    const route = buildAutomationTemplateRoute({
      template_name: 'Deploy Nginx',
    })

    expect(route).toEqual({
      path: '/sys/automation/templates',
      query: {
        search: 'Deploy Nginx',
        type: 'playbook',
      },
    })
  })

  it('builds template route with shell type when task uses shell template', () => {
    const route = buildAutomationTemplateRoute({
      template_name: '[ShellScript] check-disk',
      shell_script_template: 9,
    })

    expect(route).toEqual({
      path: '/sys/automation/templates',
      query: {
        search: 'check-disk',
        type: 'shell_script',
      },
    })
  })

  it('builds template route with playbook type when template name is blank', () => {
    const route = buildAutomationTemplateRoute({
      template_name: '   ',
    })

    expect(route).toEqual({
      path: '/sys/automation/templates',
      query: {
        type: 'playbook',
      },
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
