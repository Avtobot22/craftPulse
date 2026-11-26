import type { ReportPointFocus } from '@/shared/config'
import type { ReportSeries, RevisionMetadata, TeamReport } from '@/shared/api'

const titles: Readonly<Record<string, string>> = {
  T04: 'Начатые задачи',
  T05: 'Впервые завершённые задачи',
  T07: 'Отмены и дубликаты',
  T08: 'Длительность цикла',
  T10: 'Соблюдение целевых сроков',
  T11: 'Превышения целевых сроков',
  T14: 'Возраст текущего эпизода',
  T15: 'Время блокировок',
  T23: 'Переоткрытия workflow',
  T12: 'Задачи в работе (WIP)',
  T13: 'Заблокированные задачи внутри WIP',
  T18: 'Баланс потока',
  T38: 'Полнота полей',
  T39: 'Полнота истории',
  T40: 'Свежесть данных',
  T41: 'Покрытие сопоставления',
  T42: 'Расхождения при сверке',
}

export const outcomeReasons: Readonly<Record<string, string>> = {
  cancelled: 'Отменены',
  duplicate: 'Признаны дубликатами',
  other: 'Другая или неизвестная причина',
}

const estimators: Readonly<Record<string, string>> = {
  'T38:size': 'Размер',
  'T38:type': 'Тип',
  'T38:team': 'Команда',
  'T38:start': 'Начало работы',
  'T38:epic': 'Эпик',
  'T40:lag': 'Задержка данных',
  'T40:last_success_age': 'Давность последней успешной синхронизации',
  'T41:tasks': 'Задачи',
  'T41:transitions': 'Переходы',
  'T42:missing': 'Пропущены',
  'T42:changed': 'Изменены',
}

export function seriesTitle(series: ReportSeries, groupBy: string) {
  const { metric_id, estimator, group } = series
  const title = titles[metric_id] ?? metric_id
  const label =
    estimator &&
    (estimators[`${metric_id}:${estimator}`] ??
      (metric_id === 'T07' ? outcomeReasons[estimator] : undefined) ??
      estimator)
  const groupLabel =
    groupBy === 'size_label'
      ? (group?.size_label ?? 'Без размера')
      : (group?.task_type ?? 'Тип неизвестен')
  return [title, label, group ? groupLabel : undefined]
    .filter(Boolean)
    .join(' · ')
}

export function seriesKey(series: ReportSeries) {
  return JSON.stringify([series.metric_id, series.estimator, series.group])
}

export function latestPoint(series: ReportSeries) {
  return series.points.reduce<ReportSeries['points'][number] | undefined>(
    (latest, point) =>
      !latest || Date.parse(point.point_start) > Date.parse(latest.point_start)
        ? point
        : latest,
    undefined,
  )
}

export interface ReportBlockProps {
  report: TeamReport
  detailsCacheKey?: string
  metadata: RevisionMetadata | undefined
  groupBy: string
  focus: ReportPointFocus | undefined
  projectIds: string[] | undefined
  taskType: string | undefined
  size: string | undefined
  selectPoint: (
    series: ReportSeries,
    pointStart: string,
    targetId: string,
    chartSeries?: string,
    cohort?: { taskType: string | null | undefined; size: string | null },
  ) => void
}

export function formatTime(value: string, timeZone: string) {
  return new Intl.DateTimeFormat('ru-RU', {
    dateStyle: 'medium',
    timeStyle: 'medium',
    timeZone,
  }).format(new Date(value))
}
