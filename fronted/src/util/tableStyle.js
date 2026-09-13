// a-table 分页与空状态的统一风格工具
// 统一口径：pageSizeOptions=['10','20','30','50']、showSizeChanger、showQuickJumper、
// showTotal 统一为「共 X 条」，空状态统一为中文「暂无数据」。

export const TABLE_PAGE_SIZE_OPTIONS = ['10', '20', '30', '50']

export const TABLE_EMPTY_TEXT = '暂无数据'

export const tableLocale = { emptyText: TABLE_EMPTY_TEXT }

// 生成统一的 pagination 配置；total/current/pageSize 由调用方通过响应式对象维护
export function createPagination(total = 0, pageSize = 10, extra = {}) {
  return {
    total,
    current: 1,
    pageSize,
    showSizeChanger: true,
    pageSizeOptions: TABLE_PAGE_SIZE_OPTIONS,
    showQuickJumper: true,
    showTotal: (t) => `共 ${t} 条记录`,
    ...extra,
  }
}
