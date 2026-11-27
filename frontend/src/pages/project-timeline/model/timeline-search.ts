import type { ProjectTimelineParams, TimelineEvent } from '@/shared/api'
import { contractLabel } from '@/shared/config'

type Filters = Pick<
  ProjectTimelineParams,
  'actor_kind' | 'object_kind' | 'revision_id' | 'limit'
>

export const actors = {
  human: 'Люди',
  bot: 'Боты',
  system: 'Системы',
  unknown: 'Неизвестные участники',
} satisfies Record<NonNullable<Filters['actor_kind']>, string>

export const objects = {
  task: 'Задачи',
  merge_request: 'MR',
  pipeline: 'Запуски CI',
  document: 'Документы',
} satisfies Record<NonNullable<Filters['object_kind']>, string>

const objectNames = {
  task: 'Задача',
  merge_request: 'MR',
  pipeline: 'Pipeline',
  attempt: 'Попытка CI',
  document: 'Документ',
}

export const pageSizes = [25, 50, 100] as const
export const uuid = /^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/iu

export function objectLabel(object: TimelineEvent['object']) {
  const name = contractLabel(objectNames, object.kind)
  return [name, object.display_key, object.title].filter(Boolean).join(' · ')
}

function dayStart(date: string) {
  if (!/^\d{4}-\d{2}-\d{2}$/u.test(date)) return undefined
  const utc = new Date(date + 'T00:00:00Z')
  return Number.isFinite(+utc) && utc.toISOString().startsWith(date)
    ? utc
    : undefined
}

function instant(value: string) {
  const match =
    /^(\d{4}-\d{2}-\d{2})T(?:[01]\d|2[0-3]):[0-5]\d(?::[0-5]\d(?:\.\d+)?)?(?:Z|[+-](?:[01]\d|2[0-3]):[0-5]\d)$/u.exec(
      value,
    )
  return match?.[1] && dayStart(match[1]) && Number.isFinite(Date.parse(value))
    ? new Date(value)
    : undefined
}

export function dayWindow(date: string) {
  const start = dayStart(date)
  if (!start) return undefined
  const end = new Date(+start + 86_400_000)
  if (end.getUTCFullYear() > 9999) return undefined
  return { from: start.toISOString(), to: end.toISOString() }
}

export function intervalWindow(from: string, to: string) {
  const start = instant(from)
  const end = instant(to)
  if (!start || !end || start >= end) return undefined
  return { from: start.toISOString(), to: end.toISOString() }
}

function inputInstant(value: string) {
  return instant(value)?.toISOString().slice(0, -1) ?? ''
}

export function readTimelineSearch(search: URLSearchParams, today: string) {
  const interval = search.has('from') || search.has('to')
  const date =
    search.get('date') ??
    instant(search.get('as_of') ?? '')
      ?.toISOString()
      .slice(0, 10) ??
    today
  const from = search.get('from') ?? ''
  const to = search.get('to') ?? ''
  const period = interval ? intervalWindow(from, to) : dayWindow(date)
  const actor = (Object.keys(actors) as (keyof typeof actors)[]).find(
    (kind) => kind === search.get('actor_kind'),
  )
  const object = (Object.keys(objects) as (keyof typeof objects)[]).find(
    (kind) => kind === search.get('object_kind'),
  )
  const revision = search.get('revision_id') ?? ''
  const revisionValid = revision === '' || uuid.test(revision)
  const limit =
    pageSizes.find((size) => String(size) === search.get('limit')) ?? 25
  const cursor = search.get('cursor')
  const params: ProjectTimelineParams | undefined =
    period && revisionValid
      ? {
          ...period,
          limit,
          ...(actor ? { actor_kind: actor } : {}),
          ...(object ? { object_kind: object } : {}),
          ...(revision ? { revision_id: revision.toLowerCase() } : {}),
          ...(cursor ? { cursor } : {}),
        }
      : undefined
  return {
    params,
    draft: {
      mode: interval ? 'interval' : 'day',
      date: dayStart(date) ? date : '',
      from: inputInstant(from),
      to: inputInstant(to),
      actor: actor ?? '',
      object: object ?? '',
      revision,
      limit: String(limit),
    } as const,
    issue: !period
      ? interval
        ? 'Укажите начало и конец интервала. Конец должен быть позже начала.'
        : 'Укажите существующую дату.'
      : !revisionValid
        ? 'Укажите UUID ревизии.'
        : '',
  }
}

export type TimelineSearch = ReturnType<typeof readTimelineSearch>
