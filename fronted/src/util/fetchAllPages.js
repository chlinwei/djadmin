/**
 * 拉取分页接口的全部数据：先取第 1 页拿到 totalPages，再并发取剩余页。
 * 后端分页响应固定为 { results, totalPages, ... }，见 API_RULES.md。
 */
export async function fetchAllPages(loader, params = {}, pageSize = 100) {
  const firstResponse = await loader({ ...params, page: 1, page_size: pageSize })
  const firstData = firstResponse?.data?.data || {}
  const records = [...(firstData.results || [])]
  const totalPages = Number(firstData.totalPages || 1)
  if (totalPages > 1) {
    const responses = await Promise.all(
      Array.from({ length: totalPages - 1 }, (_, index) => loader({ ...params, page: index + 2, page_size: pageSize })),
    )
    for (const response of responses) records.push(...(response?.data?.data?.results || []))
  }
  return records
}
