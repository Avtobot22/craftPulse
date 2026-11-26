import {
  InvalidResponse,
  readArray,
  readBoolean,
  readEnum,
  readNullableString,
  readRecord,
  readString,
} from '@/shared/api'
import type { ApiClient, components } from '@/shared/api'
import { isDateTime } from '../model/report-params'
import type { ReportQuery } from '../model/report-params'

type Schemas = components['schemas']
export type ReportSeries = Schemas['MetricSeries']
type MetricValue = Schemas['MetricValue']

function readNumber(raw: unknown): number {
  if (typeof raw !== 'number' || !Number.isFinite(raw))
    throw new InvalidResponse()
  return raw
}

function nullableNumber(raw: unknown) {
  return raw === null ? null : readNumber(raw)
}

function readDate(raw: unknown) {
  const value = readString(raw)
  if (!isDateTime(value)) throw new InvalidResponse()
  return value
}

function nullableDate(raw: unknown) {
  return raw === null ? null : readDate(raw)
}

function readMetricValue(value: unknown): MetricValue {
  const raw = readRecord(value)
  const bound = raw.bound === null ? null : readRecord(raw.bound)
  const state = readEnum(raw.state, [
    'ASSESSED',
    'PROVISIONAL',
    'BOUNDED',
    'UNKNOWN',
    'NOT_EVALUATED',
    'NOT_APPLICABLE',
  ])
  if (bound && bound.kind !== 'lower' && bound.kind !== 'upper')
    throw new InvalidResponse()
  return {
    state: state === 'unknown' ? 'UNKNOWN' : state,
    value: nullableNumber(raw.value),
    unit: readString(raw.unit),
    numerator: nullableNumber(raw.numerator),
    denominator: nullableNumber(raw.denominator),
    n: nullableNumber(raw.n),
    bound: bound
      ? {
          kind: bound.kind === 'lower' ? 'lower' : 'upper',
          value: readNumber(bound.value),
        }
      : null,
    low_sample: readBoolean(raw.low_sample),
    excluded_count: readNumber(raw.excluded_count),
    missing_count: readNumber(raw.missing_count),
    reason_code: readNullableString(raw.reason_code),
  }
}

function readSeries(value: unknown): ReportSeries {
  const raw = readRecord(value)
  const group = raw.group === undefined ? undefined : readRecord(raw.group)
  return {
    metric_id: readString(raw.metric_id),
    definition_version: readNumber(raw.definition_version),
    unit: readString(raw.unit),
    points: readArray(raw.points, (item) => {
      const point = readRecord(item)
      return {
        point_start: readDate(point.point_start),
        value: readMetricValue(point.value),
      }
    }),
    ...(raw.estimator === undefined
      ? {}
      : { estimator: readString(raw.estimator) }),
    ...(group === undefined
      ? {}
      : {
          group: {
            ...(group.task_type === undefined
              ? {}
              : { task_type: readNullableString(group.task_type) }),
            ...(group.size_label === undefined
              ? {}
              : { size_label: readNullableString(group.size_label) }),
          },
        }),
  }
}

function readReport(value: unknown) {
  const raw = readRecord(value)
  const revision = readRecord(raw.revision)
  const period = readRecord(raw.period)
  return {
    team_id: readString(raw.team_id),
    period: { from: readDate(period.from), to: readDate(period.to) },
    step: readEnum(raw.step, ['day', 'week', 'month']),
    revision: {
      id: readString(revision.id),
      series_id: readString(revision.series_id),
      revision_no: readNumber(revision.revision_no),
      as_of: readDate(revision.as_of),
      published_at: nullableDate(revision.published_at),
      loading: readBoolean(revision.loading),
      irreproducible_reason: readNullableString(revision.irreproducible_reason),
    },
    grouping: readEnum(raw.grouping, [
      'current_structure',
      'structure_at_date',
    ]),
    metrics: readArray(raw.metrics, readSeries),
    access_limited: readBoolean(raw.access_limited),
    sourceAccessLost: readArray(raw.access_notes, (note) =>
      readString(readRecord(note).code),
    ).includes('source_access_lost'),
  }
}

function readRevisionMetadata(value: unknown) {
  const raw = readRecord(value)
  const timeZone = readString(raw.timezone)
  // Intl throws for an unsupported time zone; ApiClient converts it to a read error.
  new Intl.DateTimeFormat('ru-RU', { timeZone })
  return {
    id: readString(raw.id),
    timeZone,
    hasMapping: readArray(raw.inputs, (item) =>
      readString(readRecord(item).input_kind),
    ).includes('mapping'),
    watermarks: readArray(raw.source_watermarks, (item) =>
      nullableDate(readRecord(item).watermark),
    ),
  }
}

export type TeamReport = ReturnType<typeof readReport>
export type RevisionMetadata = ReturnType<typeof readRevisionMetadata>

export class RevisionNotPublished extends Error {}

export async function loadReport(
  api: ApiClient,
  teamId: string,
  query: ReportQuery,
  signal: AbortSignal,
) {
  const { data } = await api.read('/teams/{team_id}/report', readReport, {
    pathParams: { team_id: teamId },
    query,
    signal,
  })
  if (
    data.team_id !== teamId ||
    (query.revision_id && query.revision_id !== data.revision.id)
  )
    throw new InvalidResponse()
  if (!data.revision.published_at) throw new RevisionNotPublished()
  return data
}

export async function loadRevisionMetadata(
  api: ApiClient,
  revisionId: string,
  signal: AbortSignal,
) {
  const { data } = await api.read(
    '/revisions/{revision_id}',
    readRevisionMetadata,
    {
      pathParams: { revision_id: revisionId },
      signal,
    },
  )
  if (data.id !== revisionId) throw new InvalidResponse()
  return data
}

export function refreshReport(api: ApiClient, seriesId: string) {
  return api.startSeriesRefresh(seriesId, (value) => {
    const raw = readRecord(value)
    const status = readEnum(raw.status, [
      'queued',
      'running',
      'succeeded',
      'failed',
    ])
    if (raw.kind !== 'series_refresh' || status === 'unknown')
      throw new InvalidResponse()
    return status
  })
}

export interface OutcomeFilters {
  projectIds: string[] | undefined
  taskType?: string | null | undefined
  size?: string | null | undefined
}

interface OutcomeReasonPage {
  reasons: (string | null)[]
  next: string | null
}

export async function loadOutcomeReasons(
  api: ApiClient,
  revisionId: string,
  pointStart: string,
  filters: OutcomeFilters,
  signal: AbortSignal,
) {
  const reasons = new Map<string, number>()
  const seen = new Set<string>()
  let cursor: string | null = null
  do {
    const page: OutcomeReasonPage = (
      await api.read<OutcomeReasonPage>(
        '/revisions/{revision_id}/cohort',
        (value) => {
          const raw = readRecord(value)
          return {
            reasons: readArray(raw.data, (item) => {
              const row = readRecord(item)
              if (readBoolean(row.basis_deleted)) return null
              if (
                filters.taskType !== undefined &&
                (row.task_type ?? null) !== filters.taskType
              )
                return null
              if (
                filters.size !== undefined &&
                (row.size_label ?? null) !== filters.size
              )
                return null
              const reason = readNullableString(row.reason_code)
              return reason === 'cancelled' || reason === 'duplicate'
                ? reason
                : 'other'
            }),
            next: readNullableString(raw.next_cursor),
          }
        },
        {
          query: {
            metric_id: 'T07',
            point_start: pointStart,
            role: 'numerator',
            project_id: filters.projectIds,
            cursor: cursor ?? undefined,
          },
          pathParams: { revision_id: revisionId },
          signal,
        },
      )
    ).data
    for (const reason of page.reasons) {
      if (reason) reasons.set(reason, (reasons.get(reason) ?? 0) + 1)
    }
    cursor = page.next
    if (cursor && seen.has(cursor)) throw new InvalidResponse()
    if (cursor) seen.add(cursor)
  } while (cursor)
  return [...reasons]
}
