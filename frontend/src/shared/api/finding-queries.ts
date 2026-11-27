import { infiniteQueryOptions, queryOptions } from '@tanstack/react-query'
import type { ApiClient } from './client'
import type { components, operations } from './generated/schema'
import {
  readAccessNotes,
  readLocation,
  readScopedPage,
  readNullableNumber,
  readStringMap,
} from './read-fields'
import type { OpenCodes } from './read-fields'
import {
  readArray,
  readBoolean,
  readNullableString,
  readRecord,
  readString,
} from './response'

type Schema = components['schemas']
export type FindingListParams = NonNullable<
  operations['listFindings']['parameters']['query']
>
export type TechnicalComparisonParams =
  operations['compareTechnicalResults']['parameters']['query']

function readFinding(value: unknown): OpenCodes<Schema['Finding']> {
  const raw = readRecord(value)
  const severity = readRecord(raw.severity)
  const pkg = raw.package === null ? null : readRecord(raw.package)
  return {
    id: readString(raw.id),
    finding_kind: readString(raw.finding_kind),
    rule_id: readString(raw.rule_id),
    state: readString(raw.state),
    severity: {
      native: readNullableString(severity.native),
      mapped: readNullableString(severity.mapped),
      mapping_version: readNullableNumber(severity.mapping_version),
    },
    confidence: readNullableString(raw.confidence),
    location: readLocation(raw.location),
    package: pkg && {
      purl: readString(pkg.purl),
      version: readNullableString(pkg.version),
    },
    component_id: readNullableString(raw.component_id),
    first_seen_at: readString(raw.first_seen_at),
  }
}

function readFindingPage(value: unknown): OpenCodes<Schema['FindingPage']> {
  const raw = readRecord(value)
  return { ...readScopedPage(raw), data: readArray(raw.data, readFinding) }
}

function readFindingDetail(value: unknown): OpenCodes<Schema['FindingDetail']> {
  const raw = readRecord(value)
  const report = readRecord(raw.report)
  const finding = readFinding(raw)
  return {
    ...finding,
    ...(raw.area_id === undefined ? {} : { area_id: readString(raw.area_id) }),
    ...(raw.source_url === undefined
      ? {}
      : { source_url: readString(raw.source_url) }),
    evidence:
      finding.finding_kind === 'secret' ? {} : readStringMap(raw.evidence),
    report: {
      id: readString(report.id),
      producer: readString(report.producer),
      producer_version: readString(report.producer_version),
      ...(report.profile_id === undefined
        ? {}
        : { profile_id: readNullableString(report.profile_id) }),
      ...(report.mode === undefined ? {} : { mode: readString(report.mode) }),
      rules_version: readNullableString(report.rules_version),
      db_version: readNullableString(report.db_version),
      sha: readNullableString(report.sha),
      completeness: readString(report.completeness),
    },
    external_triage: readArray(raw.external_triage, (value) => {
      const triage = readRecord(value)
      return {
        authority: readString(triage.authority),
        state: readString(triage.state),
        source_url: readNullableString(triage.source_url),
      }
    }),
    exceptions: readArray(raw.exceptions, (value) => {
      const exception = readRecord(value)
      return {
        exception_id: readString(exception.exception_id),
        state: readString(exception.state),
        expires_at: readString(exception.expires_at),
      }
    }),
    actions: readArray(raw.actions, readString),
    redactions: readArray(raw.redactions, readString),
  }
}

function readComparedReport(value: unknown) {
  const raw = readRecord(value)
  return {
    sha: readString(raw.sha),
    report_id: readNullableString(raw.report_id),
  }
}

function readComparison(
  value: unknown,
): OpenCodes<Schema['TechnicalComparison']> {
  const raw = readRecord(value)
  return {
    comparable: readBoolean(raw.comparable),
    base: readComparedReport(raw.base),
    head: readComparedReport(raw.head),
    incomparable_reasons: readArray(raw.incomparable_reasons, readString),
    factors: readArray(raw.factors, (value) => {
      const factor = readRecord(value)
      return {
        factor: readString(factor.factor),
        new_count: readNullableNumber(factor.new_count),
        resolved_count: readNullableNumber(factor.resolved_count),
        finding_ids: readArray(factor.finding_ids, readString),
      }
    }),
    access_limited: readBoolean(raw.access_limited),
    access_notes: readAccessNotes(raw.access_notes),
  }
}

async function fetchFindings(
  api: ApiClient,
  params: FindingListParams,
  signal: AbortSignal,
) {
  return (
    await api.read('/findings', readFindingPage, { query: params, signal })
  ).data
}

export function listFindings(api: ApiClient, params: FindingListParams = {}) {
  return queryOptions({
    queryKey: ['findings', 'list', params],
    queryFn: ({ signal }) => fetchFindings(api, params, signal),
  })
}

export function findingPages(api: ApiClient, params: FindingListParams) {
  return infiniteQueryOptions({
    queryKey: ['findings', 'pages', params],
    initialPageParam: params.cursor ?? '',
    queryFn: ({ signal, pageParam }) =>
      fetchFindings(
        api,
        pageParam ? { ...params, cursor: pageParam } : params,
        signal,
      ),
    getNextPageParam: ({ next_cursor }, _pages, _param, pageParams) =>
      next_cursor && !pageParams.includes(next_cursor)
        ? next_cursor
        : undefined,
  })
}

export function getFinding(
  api: ApiClient,
  findingId: operations['getFinding']['parameters']['path']['finding_id'],
) {
  return queryOptions({
    queryKey: ['findings', 'detail', findingId],
    queryFn: async ({ signal }) =>
      (
        await api.read('/findings/{finding_id}', readFindingDetail, {
          pathParams: { finding_id: findingId },
          signal,
        })
      ).data,
  })
}

export function compareTechnicalResults(
  api: ApiClient,
  params: TechnicalComparisonParams,
) {
  return queryOptions({
    queryKey: ['findings', 'comparison', params],
    queryFn: async ({ signal }) =>
      (
        await api.read('/technical-comparisons', readComparison, {
          query: params,
          signal,
        })
      ).data,
  })
}
