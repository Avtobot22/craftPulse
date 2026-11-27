import type { ReportQuery } from '@/shared/api'
import { isDateTime } from '@/shared/lib/date-time'

export interface ReportParams {
  from: string
  to: string
  step: 'week' | 'month'
  projects: string[]
  taskType: string
  size: string
  groupBy: 'task_type' | 'size_label' | ''
  revision: string
  minTaskAgeSeconds?: number | undefined
  overTarget?: boolean | undefined
}

export function readReportParams(search: URLSearchParams): ReportParams {
  const today = new Date().toISOString().slice(0, 10)
  const end = new Date(today + 'T00:00:00Z')
  const start = new Date(end)
  start.setUTCDate(start.getUTCDate() - 28)
  const grouping = search.get('group_by')
  const age = search.get('min_task_age_seconds')
  const target = search.get('over_target')
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
    minTaskAgeSeconds:
      age && /^\d+$/u.test(age) && Number.isSafeInteger(Number(age))
        ? Number(age)
        : undefined,
    overTarget:
      target === 'true' ? true : target === 'false' ? false : undefined,
  }
}

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
    ['min_task_age_seconds', params.minTaskAgeSeconds?.toString()],
    ['over_target', params.overTarget?.toString()],
  ] as const) {
    if (value) next.set(key, value)
    else next.delete(key)
  }
  return next
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
    ...technicalReportFilters(params),
  }
}

export function technicalReportFilters(params: ReportParams) {
  return {
    ...(params.minTaskAgeSeconds === undefined
      ? {}
      : { min_task_age_seconds: params.minTaskAgeSeconds }),
    ...(params.overTarget === undefined
      ? {}
      : { over_target: params.overTarget }),
  }
}

export function technicalCohortLimitMessage(
  metricId: string,
  groupBy: string,
  group: string | null,
) {
  // For pipeline and MR rows the types of tasks in basis are unavailable.
  if (
    (metricId === 'X04' || metricId === 'X06') &&
    groupBy === 'task_type' &&
    group === ''
  )
    return 'API не выделяет эту когорту для задач с неизвестным типом. Снимите группировку, чтобы увидеть список.'
  return undefined
}

export function periodError({ from, to }: ReportParams) {
  if (!isDateTime(from) || !isDateTime(to))
    return 'Укажите начало и конец периода.'
  if (Date.parse(from) >= Date.parse(to))
    return 'Конец периода должен быть позже начала.'
  return undefined
}
