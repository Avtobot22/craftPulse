import { isDateTime } from '@/shared/lib/date-time'
import type { ApiClient } from './client'
import type { components, operations } from './generated/schema'
import { readNumber, readNullableNumber } from './read-fields'
import {
  InvalidResponse,
  readArray,
  readBoolean,
  readEnum,
  readNullableString,
  readRecord,
  readString,
} from './response'

export type ReportQuery = operations['getTeamReport']['parameters']['query']

type Schemas = components['schemas']
export type ReportSeries = Schemas['MetricSeries']
type MetricValue = Schemas['MetricValue']

export function readDate(raw: unknown) {
  const value = readString(raw)
  if (!isDateTime(value)) throw new InvalidResponse()
  return value
}

export function nullableDate(raw: unknown) {
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
    value: readNullableNumber(raw.value),
    unit: readString(raw.unit),
    numerator: readNullableNumber(raw.numerator),
    denominator: readNullableNumber(raw.denominator),
    n: readNullableNumber(raw.n),
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

export function readReportSeries(value: unknown): ReportSeries {
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

export function readReport(value: unknown) {
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
    metrics: readArray(raw.metrics, readReportSeries),
    access_limited: readBoolean(raw.access_limited),
    sourceAccessLost: readArray(raw.access_notes, (note) =>
      readString(readRecord(note).code),
    ).includes('source_access_lost'),
  }
}

export type TeamReport = ReturnType<typeof readReport>

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
