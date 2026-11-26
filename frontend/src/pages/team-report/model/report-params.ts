import type { ReportParams } from '@/shared/config'

export function writeReportParams(
  search: URLSearchParams,
  params: ReportParams,
) {
  const next = new URLSearchParams(search)
  next.set('from', params.from)
  next.set('to', params.to)
  next.set('step', params.step)
  next.delete('report_project_id')
  for (const id of params.projects) next.append('report_project_id', id)
  for (const [key, value] of [
    ['task_type', params.taskType],
    ['size_label', params.size],
    ['group_by', params.groupBy],
    ['revision_id', params.revision],
  ] as const) {
    if (value) next.set(key, value)
    else next.delete(key)
  }
  return next
}
