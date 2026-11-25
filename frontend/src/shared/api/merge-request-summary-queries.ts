import { queryOptions } from '@tanstack/react-query'
import type { ApiClient } from './client'
import type { components, operations } from './generated/schema'
import type { OpenCodes } from './read-fields'
import {
  readAccessNotes,
  readLocation,
  readMetricValue,
  readNullableNumber,
  readNumber,
} from './read-fields'
import {
  InvalidResponse,
  readArray,
  readBoolean,
  readNullableString,
  readRecord,
  readString,
} from './response'

type Schema = components['schemas']
type Summary = OpenCodes<Schema['MergeRequestSummary']>
export type MergeRequestSummaryParams = NonNullable<
  operations['getMergeRequestSummary']['parameters']['query']
>

function readSummaryItem(value: unknown): OpenCodes<Schema['SummaryItem']> {
  const item = readRecord(value)
  return {
    item_no: readNumber(item.item_no),
    item_kind: readString(item.item_kind),
    ...(item.analysis_kind === undefined
      ? {}
      : { analysis_kind: readString(item.analysis_kind) }),
    ...(item.mode === undefined ? {} : { mode: readString(item.mode) }),
    finding_id: readNullableString(item.finding_id),
    rule_id: readNullableString(item.rule_id),
    severity: readNullableString(item.severity),
    location: readLocation(item.location),
    rank: readNullableNumber(item.rank),
  }
}

function readSummaryCI(value: unknown): Summary['ci'] {
  const ci = value === null ? null : readRecord(value)
  return (
    ci && {
      pipeline_id: readString(ci.pipeline_id),
      outcome: readString(ci.outcome),
      attempts: readNumber(ci.attempts),
      retried_jobs: readNumber(ci.retried_jobs),
      ...(ci.parent_pipeline_id === undefined
        ? {}
        : { parent_pipeline_id: readNullableString(ci.parent_pipeline_id) }),
    }
  )
}

function readSummaryExtras(
  raw: Record<string, unknown>,
): Pick<Summary, 'tests' | 'change' | 'expected' | 'last_good'> {
  const extras: Pick<Summary, 'tests' | 'change' | 'expected' | 'last_good'> =
    {}
  if (raw.tests !== undefined) {
    const tests = readRecord(raw.tests)
    extras.tests = {
      passed: readMetricValue(tests.passed),
      failed: readMetricValue(tests.failed),
      errored: readMetricValue(tests.errored),
      skipped: readMetricValue(tests.skipped),
    }
  }
  if (raw.change !== undefined) {
    const change = readRecord(raw.change)
    extras.change = {
      lines_added: readMetricValue(change.lines_added),
      lines_removed: readMetricValue(change.lines_removed),
      files_changed: readMetricValue(change.files_changed),
      complexity_increased_functions: readMetricValue(
        change.complexity_increased_functions,
      ),
      new_clone_groups: readMetricValue(change.new_clone_groups),
    }
  }
  if (raw.expected !== undefined) {
    const expected = readRecord(raw.expected)
    extras.expected = {
      missing_checks: readMetricValue(expected.missing_checks),
      missing_reports: readMetricValue(expected.missing_reports),
    }
  }
  if (raw.last_good !== undefined) {
    const lastGood = raw.last_good === null ? null : readRecord(raw.last_good)
    extras.last_good = lastGood && {
      head_sha: readString(lastGood.head_sha),
      revision_no: readNumber(lastGood.revision_no),
      built_at: readString(lastGood.built_at),
    }
  }
  return extras
}

function readSummary(value: unknown): Summary {
  const raw = readRecord(value)
  const publication = readRecord(raw.publication)
  const revision = readNumber(raw.revision_no)
  if (!Number.isSafeInteger(revision)) throw new InvalidResponse()
  return {
    id: readString(raw.id),
    merge_request_id: readString(raw.merge_request_id),
    revision_no: revision,
    status: readString(raw.status),
    head_sha: readString(raw.head_sha),
    base_sha: readNullableString(raw.base_sha),
    pipeline_state: readString(raw.pipeline_state),
    built_at: readNullableString(raw.built_at),
    stale_reason: readNullableString(raw.stale_reason),
    items: readArray(raw.items, readSummaryItem),
    not_evaluated: readArray(raw.not_evaluated, (value) => {
      const item = readRecord(value)
      return {
        analysis_kind: readString(item.analysis_kind),
        reason_code: readString(item.reason_code),
      }
    }),
    coverage_changed_lines: readMetricValue(raw.coverage_changed_lines),
    ...readSummaryExtras(raw),
    ci: readSummaryCI(raw.ci),
    publication: {
      status: readString(publication.status),
      external_id: readNullableString(publication.external_id),
      error_code: readNullableString(publication.error_code),
      published_at: readNullableString(publication.published_at),
    },
    redactions: readArray(raw.redactions, readString),
    access_limited: readBoolean(raw.access_limited),
    access_notes: readAccessNotes(raw.access_notes),
  }
}

export function getMergeRequestSummary(
  api: ApiClient,
  mrId: operations['getMergeRequestSummary']['parameters']['path']['merge_request_id'],
  params: MergeRequestSummaryParams = {},
) {
  return queryOptions({
    queryKey: ['merge-request-summary', mrId, params.revision_no],
    queryFn: async ({ signal }) => {
      const { data } = await api.read(
        '/merge-requests/{merge_request_id}/summary',
        readSummary,
        {
          pathParams: { merge_request_id: mrId },
          query: params,
          signal,
        },
      )
      if (
        data.merge_request_id !== mrId ||
        (params.revision_no !== undefined &&
          data.revision_no !== params.revision_no)
      )
        throw new InvalidResponse()
      return data
    },
    retry: false,
  })
}
