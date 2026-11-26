import {
  InvalidResponse,
  readArray,
  readBoolean,
  readEnum,
  readNullableString,
  readRecord,
  readString,
} from './response'
import type { ApiClient } from './client'
import type { components, operations } from './generated/schema'
import { readNullableNumber, readNumber } from './read-fields'

type Schemas = components['schemas']
export type ReportQuery = operations['getTeamReport']['parameters']['query']
export type ReportSeries = Schemas['MetricSeries']
type MetricValue = Schemas['MetricValue']

const dateTime =
  /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/u

export function isDateTime(value: string) {
  return dateTime.test(value) && Number.isFinite(Date.parse(value))
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
  const inputs = readArray(raw.inputs, (item) => {
    const input = readRecord(item)
    return {
      kind: readString(input.input_kind),
      id: readString(input.input_id),
      version: readNullableString(input.version),
    }
  })
  const definitions = readArray(raw.definitions, (item) => {
    const definition = readRecord(item)
    const metricId = readString(definition.metric_id)
    const version = readNumber(definition.definition_version)
    const days =
      metricId === 'T23' && definition.parameters !== undefined
        ? readRecord(definition.parameters).follow_up_days
        : undefined
    return {
      metricId,
      version,
      name: readString(definition.name),
      followUpDays:
        typeof days === 'number' && Number.isInteger(days) && days > 0
          ? days
          : undefined,
    }
  })
  const window = readRecord(raw.window)
  return {
    id: readString(raw.id),
    number: readNumber(raw.revision_no),
    asOf: readDate(raw.as_of),
    window: { from: readDate(window.from), to: readDate(window.to) },
    timeZone,
    definitions,
    inputs,
    irreproducibleReason: readNullableString(raw.irreproducible_reason),
    reopenWindows: Object.fromEntries(
      definitions.flatMap(({ version, followUpDays }) =>
        followUpDays === undefined ? [] : [[version, followUpDays] as const],
      ),
    ),
    hasMapping: inputs.some(({ kind }) => kind === 'mapping'),
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
