import { checkPermission } from '@/directives/permission/permission'

export const canViewBusiness = () => checkPermission('assets:service-tree:view')
export const canManageTree = () => checkPermission('assets:service-tree:manage')
export const canViewService = () => checkPermission('assets:applications:view')
// The tree's management entry must not grant the application API's action permission.
export const canManageService = (action) => canManageTree() && checkPermission(`assets:applications:${action}`)
