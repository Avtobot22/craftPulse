import type { ApiClient } from './client'
import type { components } from './generated/schema'
import { readNumber } from './read-fields'
import { nullableDate, readDate, readReportSeries } from './report-queries'
import {
  InvalidResponse,
  readArray,
  readBoolean,
  readNullableString,
  readRecord,
  readString,
} from './response'

type Schemas = components['schemas']
export type MetricDefinition = Schemas['MetricDefinition']

function readDefinition(value: unknown): MetricDefinition {
  const raw = readRecord(value)
  return {
    metric_id: readString(raw.metric_id),
    definition_version: readNumber(raw.definition_version),
    name: readString(raw.name),
    unit: readString(raw.unit),
    catalog_ref: readString(raw.catalog_ref),
    ...(raw.parameters === undefined
      ? {}
      : { parameters: readRecord(raw.parameters) }),
  }
}

function readRevision(value: unknown) {
  const raw = readRecord(value)
  const window = readRecord(raw.window)
  const timezone = readString(raw.timezone)
  new Intl.DateTimeFormat('ru-RU', { timeZone: timezone })

  return {
    id: readString(raw.id),
    series_id: readString(raw.series_id),
    revision_no: readNumber(raw.revision_no),
    status: readString(raw.status),
    window: { from: readDate(window.from), to: readDate(window.to) },
    timezone,
    as_of: readDate(raw.as_of),
    knowledge_seq: readNumber(raw.knowledge_seq),
    published_at: nullableDate(raw.published_at),
    superseded_at: nullableDate(raw.superseded_at),
    loading: readBoolean(raw.loading),
    irreproducible_reason: readNullableString(raw.irreproducible_reason),
    definitions: readArray(raw.definitions, readDefinition),
    inputs: readArray(raw.inputs, (value) => {
      const input = readRecord(value)
      return {
        input_kind: readString(input.input_kind),
        input_id: readString(input.input_id),
        version: readNullableString(input.version),
      }
    }),
    source_watermarks: readArray(raw.source_watermarks, (value) => {
      const source = readRecord(value)
      return {
        connection_id: readString(source.connection_id),
        watermark: nullableDate(source.watermark),
        completeness: readString(source.completeness),
      }
    }),
  }
}

export type MetricRevision = ReturnType<typeof readRevision>

export async function loadMetricRevision(
  api: ApiClient,
  revisionId: string,
  signal: AbortSignal,
) {
  const { data } = await api.read('/revisions/{revision_id}', readRevision, {
    pathParams: { revision_id: revisionId },
    signal,
  })
  if (data.id !== revisionId) throw new InvalidResponse()
  return data
}

export function followUpDays(definition: MetricDefinition) {
  const days = definition.parameters?.follow_up_days
  return definition.metric_id === 'T23' &&
    typeof days === 'number' &&
    Number.isInteger(days) &&
    days > 0
    ? days
    : undefined
}

export type RevisionMetadata = Awaited<ReturnType<typeof loadRevisionMetadata>>

export async function loadRevisionMetadata(
  api: ApiClient,
  revisionId: string,
  signal: AbortSignal,
) {
  const revision = await loadMetricRevision(api, revisionId, signal)
  return {
    id: revision.id,
    timeZone: revision.timezone,
    hasMapping: revision.inputs.some(
      ({ input_kind }) => input_kind === 'mapping',
    ),
    watermarks: revision.source_watermarks.map(({ watermark }) => watermark),
    reopenWindows: Object.fromEntries(
      revision.definitions.flatMap((definition) => {
        const days = followUpDays(definition)
        return days === undefined
          ? []
          : [[definition.definition_version, days] as const]
      }),
    ),
  }
}

export async function findMetricDefinition(
  api: ApiClient,
  metricId: string,
  version: number | undefined,
  signal: AbortSignal,
) {
  const seen = new Set<string>()
  let cursor: string | undefined
  do {
    const { data: page } = await api.read(
      '/metric-definitions',
      (value) => {
        const raw = readRecord(value)
        return {
          data: readArray(raw.data, readDefinition),
          next: readNullableString(raw.next_cursor),
        }
      },
      { query: { cursor, limit: 100 }, signal },
    )
    const match = page.data.find(
      (entry) =>
        entry.metric_id === metricId &&
        (version === undefined || entry.definition_version === version),
    )
    if (match) return match
    cursor = page.next ?? undefined
    if (cursor) {
      if (seen.has(cursor)) throw new InvalidResponse()
      seen.add(cursor)
    }
  } while (cursor)
  return null
}

function readSeriesData(value: unknown) {
  const raw = readRecord(value)
  const revision = readRecord(raw.revision)
  return {
    seriesId: readString(raw.series_id),
    revisionId: readString(revision.id),
    publishedAt: nullableDate(revision.published_at),
    metrics: readArray(raw.metrics, readReportSeries),
    accessLimited: readBoolean(raw.access_limited),
  }
}

export async function loadRevisionSeries(
  api: ApiClient,
  seriesId: string,
  metricId: string,
  revisionId: string | undefined,
  signal: AbortSignal,
) {
  const { data } = await api.read('/series/{series_id}/data', readSeriesData, {
    pathParams: { series_id: seriesId },
    query: { metric_id: metricId, revision_id: revisionId },
    signal,
  })
  if (
    data.seriesId !== seriesId ||
    (revisionId !== undefined && data.revisionId !== revisionId)
  )
    throw new InvalidResponse()
  return data
}
