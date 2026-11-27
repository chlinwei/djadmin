import { computed } from 'vue'
import { checkPermission } from '@/directives/permission/permission'

export const canView = computed(() => checkPermission('assets:applications:view'))
export const canUpdate = computed(() => checkPermission('assets:applications:update'))
export const canCreate = computed(() => checkPermission('assets:applications:create'))
export const canDelete = computed(() => checkPermission('assets:applications:delete'))
