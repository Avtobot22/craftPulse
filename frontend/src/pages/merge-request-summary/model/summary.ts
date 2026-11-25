import {
  InvalidResponse,
  readArray,
  readBoolean,
  readNullableString,
  readRecord,
  readString,
} from '@/shared/api'
import type { components } from '@/shared/api'

type Summary = components['schemas']['MergeRequestSummary']

function readInteger(value: unknown): number {
  if (typeof value !== 'number' || !Number.isSafeInteger(value))
    throw new InvalidResponse()
  return value
}

export function readSummary(value: unknown) {
  const row = readRecord(value)
  const publication = readRecord(row.publication)
  const coverage = readRecord(row.coverage_changed_lines)
  const ci = row.ci === null ? null : readRecord(row.ci)
  const tests = row.tests === undefined ? null : readRecord(row.tests)
  const change = row.change === undefined ? null : readRecord(row.change)

  return {
    mr: readString(row.merge_request_id),
    revision: readInteger(row.revision_no),
    head: readString(row.head_sha),
    base: readNullableString(row.base_sha),
    builtAt: readNullableString(row.built_at),
    status: readString(row.status),
    pipelineState: readString(row.pipeline_state),
    staleReason: readNullableString(row.stale_reason),
    restricted: readBoolean(row.access_limited),
    publication: {
      status: readString(publication.status),
      error: readNullableString(publication.error_code),
      publishedAt: readNullableString(publication.published_at),
      externalId: readNullableString(publication.external_id),
    },
    coverage: readMetric(coverage),
    ci: ci && {
      outcome: readString(ci.outcome),
      attempts: readInteger(ci.attempts),
      retried: readInteger(ci.retried_jobs),
    },
    tests: tests && {
      passed: readMetric(tests.passed),
      failed: readMetric(tests.failed),
      errored: readMetric(tests.errored),
      skipped: readMetric(tests.skipped),
    },
    change: change && {
      added: readMetric(change.lines_added),
      removed: readMetric(change.lines_removed),
      files: readMetric(change.files_changed),
      complexity: readMetric(change.complexity_increased_functions),
      clones: readMetric(change.new_clone_groups),
    },
    missing: readArray(row.not_evaluated, (value) => {
      const item = readRecord(value)
      return {
        kind: readString(item.analysis_kind),
        reason: readString(item.reason_code),
      }
    }),
    signals: readArray(row.items, (value) => {
      const item = readRecord(value)
      const location = item.location === null ? null : readRecord(item.location)
      return {
        no: readInteger(item.item_no),
        findingId: readNullableString(item.finding_id),
        kind: readString(item.item_kind),
        rule: readNullableString(item.rule_id),
        severity: readNullableString(item.severity),
        rank: item.rank === null ? null : readInteger(item.rank),
        path: location && readString(location.path),
        line:
          location && location.line_start !== null
            ? readInteger(location.line_start)
            : null,
      }
    }).filter(({ kind }) =>
      ['new', 'resolved', 'newly_observed', 'unchanged', 'accepted'].includes(
        kind,
      ),
    ),
  }
}

function readMetric(value: unknown) {
  const point = readRecord(value)
  const amount = point.value
  if (
    amount !== null &&
    (typeof amount !== 'number' || !Number.isFinite(amount))
  )
    throw new InvalidResponse()
  return {
    value: amount,
    state: readString(point.state),
    reason: readNullableString(point.reason_code),
  }
}

export const publicationLabels: Record<
  Summary['publication']['status'],
  string
> = {
  not_requested: 'Публикация не запрошена',
  pending: 'Ожидает публикации',
  succeeded: 'Комментарий опубликован',
  failed: 'Комментарий не опубликован',
  unknown: 'Исход публикации неизвестен',
  stale: 'Публикация устарела',
}
