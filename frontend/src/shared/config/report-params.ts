import { isDateTime } from '@/shared/api'
import type { ReportQuery } from '@/shared/api'

export interface ReportParams {
  from: string
  to: string
  step: 'week' | 'month'
  projects: string[]
  taskType: string
  size: string
  groupBy: 'task_type' | 'size_label' | ''
  revision: string
}

export function readReportParams(search: URLSearchParams): ReportParams {
  const today = new Date().toISOString().slice(0, 10)
  const end = new Date(today + 'T00:00:00Z')
  const start = new Date(end)
  start.setUTCDate(start.getUTCDate() - 28)
  const grouping = search.get('group_by')
  return {
    from: search.get('from') ?? start.toISOString(),
    to: search.get('to') ?? end.toISOString(),
    step: search.get('step') === 'month' ? 'month' : 'week',
    projects: [...new Set(search.getAll('report_project_id'))],
    taskType: search.get('task_type') ?? '',
    size: search.get('size_label') ?? '',
    groupBy:
      grouping === 'task_type' || grouping === 'size_label' ? grouping : '',
    revision: search.get('revision_id') ?? '',
  }
}

export function reportQuery(
  params: ReportParams,
  scopeProject?: string,
): ReportQuery {
  const projects = scopeProject ? [scopeProject] : params.projects
  return {
    from: params.from,
    to: params.to,
    step: params.step,
    ...(projects.length ? { project_id: projects } : {}),
    ...(params.taskType ? { task_type: params.taskType } : {}),
    ...(params.size ? { size_label: params.size } : {}),
    ...(params.groupBy ? { group_by: params.groupBy } : {}),
    ...(params.revision ? { revision_id: params.revision } : {}),
  }
}

export function periodError({ from, to }: ReportParams) {
  if (!isDateTime(from) || !isDateTime(to))
    return 'Укажите начало и конец периода.'
  if (Date.parse(from) >= Date.parse(to))
    return 'Конец периода должен быть позже начала.'
  return undefined
}
