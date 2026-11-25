import {
  InvalidResponse,
  readArray,
  readBoolean,
  readEnum,
  readNullableString,
  readRecord,
  readString,
} from '@/shared/api'
import type { DataState } from '@/shared/ui'

const states = [
  'ASSESSED',
  'PROVISIONAL',
  'BOUNDED',
  'NOT_EVALUATED',
  'NOT_APPLICABLE',
  'UNKNOWN',
] as const satisfies readonly DataState[]

function readNumber(value: unknown): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    throw new InvalidResponse()
  }
  return value
}

function nullableNumber(value: unknown): number | null {
  return value === null ? null : readNumber(value)
}

function readMetric(value: unknown) {
  const metric = readRecord(value)
  const bound = metric.bound === null ? null : readRecord(metric.bound)
  const decodedState = readEnum(metric.state, states)
  const state: DataState = decodedState === 'unknown' ? 'UNKNOWN' : decodedState
  return {
    state,
    value: nullableNumber(metric.value),
    numerator: nullableNumber(metric.numerator),
    denominator: nullableNumber(metric.denominator),
    unit: readString(metric.unit),
    reason: readNullableString(metric.reason_code),
    bound: bound && {
      kind: readEnum(bound.kind, ['lower', 'upper']),
      value: readNumber(bound.value),
    },
  }
}

function readMetrics(value: unknown, labels: Readonly<Record<string, string>>) {
  if (value === undefined) return []
  const metrics = readRecord(value)
  return Object.entries<string>(labels).map(([key, label]) => ({
    key,
    label,
    metric: readMetric(metrics[key]),
  }))
}

export function readSummary(value: unknown) {
  const summary = readRecord(value)
  const ci = summary.ci === null ? null : readRecord(summary.ci)
  const publication = readRecord(summary.publication)
  return {
    head: readString(summary.head_sha),
    status: readString(summary.status),
    pipelineId: ci && readString(ci.pipeline_id),
    publication: {
      status: readEnum(publication.status, [
        'not_requested',
        'pending',
        'succeeded',
        'failed',
        'unknown',
        'stale',
      ]),
      errorCode: readNullableString(publication.error_code),
    },
    coverage: readMetric(summary.coverage_changed_lines),
    tests: readMetrics(summary.tests, {
      passed: 'Пройдено',
      failed: 'Не пройдено',
      errored: 'Завершились с ошибкой',
      skipped: 'Пропущено',
    }),
    change: readMetrics(summary.change, {
      lines_added: 'Добавлено строк',
      lines_removed: 'Удалено строк',
      files_changed: 'Изменено файлов',
      complexity_increased_functions: 'Функции с ростом сложности',
      new_clone_groups: 'Новые группы дублирования',
    }),
    expected: readMetrics(summary.expected, {
      missing_checks: 'Пропущено проверок',
      missing_reports: 'Пропущено отчётов',
    }),
    notEvaluated: readArray(summary.not_evaluated, (value) => {
      const analysis = readRecord(value)
      return {
        kind: readString(analysis.analysis_kind),
        reason: readString(analysis.reason_code),
      }
    }),
    limited: readBoolean(summary.access_limited),
    items: readArray(summary.items, (value) => {
      const item = readRecord(value)
      return {
        kind: readString(item.item_kind),
        analysis:
          item.analysis_kind === undefined
            ? null
            : readString(item.analysis_kind),
        rule: readNullableString(item.rule_id),
      }
    }),
  }
}

export type Summary = ReturnType<typeof readSummary>
