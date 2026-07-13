import { buildAutomationInventoryRoute, buildAutomationPlaybookRoute } from '../navigation'

describe('automation navigation helpers', () => {
  it('builds playbook route with search when template name exists', () => {
    const route = buildAutomationPlaybookRoute({
      template_name: 'Deploy Nginx',
    })

    expect(route).toEqual({
      path: '/sys/automation/playbooks',
      query: {
        search: 'Deploy Nginx',
      },
    })
  })

  it('builds playbook route with empty query when template name is blank', () => {
    const route = buildAutomationPlaybookRoute({
      template_name: '   ',
    })

    expect(route).toEqual({
      path: '/sys/automation/playbooks',
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
