import { Button, Card, Text } from '@gravity-ui/uikit'
import { useInfiniteQuery } from '@tanstack/react-query'
import { useLocation, useSearchParams } from 'react-router'
import { SourceLink } from '@/entities/activity'
import { testExecutionPages } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { contractLabel } from '@/shared/config'
import { DataStateBadge, ErrorState, LoadingState } from '@/shared/ui'
import type { DataState } from '@/shared/ui'
import {
  isAccessError,
  testOutcome,
  testOutcomeLabels,
  testOutcomeParam,
} from '../model/pipeline'
import type { Attempt, Pipeline } from '../model/pipeline'
import { PipelineTable } from './pipeline-table'

const percent = new Intl.NumberFormat('ru-RU', {
  style: 'percent',
  maximumFractionDigits: 2,
})

function testState(state: string): DataState {
  switch (state) {
    case 'ASSESSED':
    case 'PROVISIONAL':
    case 'BOUNDED':
    case 'NOT_APPLICABLE':
    case 'NOT_EVALUATED':
      return state
    default:
      return 'UNKNOWN'
  }
}

export function PipelineTests({
  pipeline,
  canListExecutions,
}: {
  pipeline: Pipeline
  canListExecutions: boolean
}) {
  const { state, passed, failed, error, skipped, distinct_cases, executions } =
    pipeline.tests
  const hasJUnit = pipeline.reports.some(
    (report) => report.analysis_kind === 'junit' && report.state === 'received',
  )
  const denominator =
    passed !== null && failed !== null && error !== null
      ? passed + failed + error
      : 0
  const passRate =
    hasJUnit && state === 'ASSESSED' && passed !== null && denominator > 0
      ? percent.format(passed / denominator)
      : null
  return (
    <Card className="cp-ci-panel">
      <Text as="h2" variant="subheader-2">
        Тесты
      </Text>
      <DataStateBadge state={testState(state)} />
      <dl className="cp-ci-facts">
        {Object.entries({ passed, failed, error, skipped }).map(
          ([outcome, count]) => (
            <div key={outcome}>
              <dt>{contractLabel(testOutcomeLabels, outcome)}</dt>
              <dd>{count ?? 'Нет данных'}</dd>
            </div>
          ),
        )}
        {distinct_cases != null && (
          <div>
            <dt>Уникальные тесты</dt>
            <dd>{distinct_cases}</dd>
          </div>
        )}
        {executions != null && (
          <div>
            <dt>Выполнения</dt>
            <dd>{executions}</dd>
          </div>
        )}
        {passRate && (
          <div>
            <dt>Доля успешных без пропущенных</dt>
            <dd>{passRate}</dd>
          </div>
        )}
      </dl>
      {canListExecutions && (
        <a
          href="#test-executions"
          onClick={(event) => {
            if (
              event.ctrlKey ||
              event.metaKey ||
              event.shiftKey ||
              event.altKey
            )
              return
            const heading = document.getElementById('test-executions')
            if (heading) {
              event.preventDefault()
              heading.focus()
            }
          }}
        >
          Перейти к выполнениям тестов
        </a>
      )}
    </Card>
  )
}

export function PipelineTestExecutions({
  api,
  pipelineId,
  attempts,
}: {
  api: ApiClient
  pipelineId: string
  attempts: Attempt[]
}) {
  const [params, setParams] = useSearchParams()
  const state: unknown = useLocation().state
  const filter = testOutcome(params.get(testOutcomeParam))
  const query = useInfiniteQuery(testExecutionPages(api, pipelineId, filter))
  const byId = new Map(attempts.map((attempt) => [attempt.id, attempt]))

  function selectOutcome(value: string) {
    const next = new URLSearchParams(params)
    if (value) next.set(testOutcomeParam, value)
    else next.delete(testOutcomeParam)
    setParams(next, { replace: true, state })
  }

  const denied = isAccessError(query.error)
  const rows = denied
    ? []
    : (query.data?.pages.flatMap(({ data }, page) =>
        data.map((execution, index) => {
          const attempt = execution.attempt_id
            ? byId.get(execution.attempt_id)
            : undefined
          const attemptLabel = attempt
            ? `№ ${String(attempt.attempt_no)} · ${attempt.job_key}`
            : (execution.attempt_id ?? 'Нет связи')
          return {
            // The API does not return an execution ID. Even identical rows can
            // represent separate report executions.
            key: `${String(page)}:${String(index)}`,
            cells: [
              <code key="case">{execution.test_case.key}</code>,
              <code key="config">{execution.config_fingerprint}</code>,
              <SourceLink key="attempt" url={attempt?.source_url ?? null}>
                {attemptLabel}
              </SourceLink>,
              contractLabel(testOutcomeLabels, execution.outcome),
              execution.duration_ms === null
                ? 'Неизвестно'
                : `${String(execution.duration_ms)} мс`,
            ],
          }
        }),
      ) ?? [])

  return (
    <Card className="cp-ci-panel">
      <h2 id="test-executions" tabIndex={-1}>
        Выполнения тестов
      </h2>
      <label className="cp-ci-filter">
        Исход теста
        <select
          value={filter ?? ''}
          onChange={(event) => {
            selectOutcome(event.target.value)
          }}
        >
          <option value="">Все исходы</option>
          {Object.entries(testOutcomeLabels).map(([outcome, label]) => (
            <option key={outcome} value={outcome}>
              {label}
            </option>
          ))}
        </select>
      </label>
      {query.isPending && <LoadingState />}
      {denied ? (
        <p>Выполнения тестов недоступны.</p>
      ) : (
        <>
          {query.isError && (
            <ErrorState
              reason="Не удалось загрузить выполнения тестов"
              retrying={query.isFetching}
              onRetry={() => {
                if (query.isFetchNextPageError)
                  void query.fetchNextPage({ cancelRefetch: false })
                else void query.refetch()
              }}
            />
          )}
          {rows.length > 0 && (
            <PipelineTable
              label="Выполнения тестов"
              columns={[
                'Тест',
                'Конфигурация',
                'Попытка',
                'Исход',
                'Длительность',
              ]}
              rows={rows}
            />
          )}
          {query.isSuccess && rows.length === 0 && !query.hasNextPage && (
            <p>
              {filter
                ? 'Нет выполнений с выбранным исходом.'
                : 'Нет данных о выполнениях тестов.'}
            </p>
          )}
          {query.hasNextPage && !query.isFetchNextPageError && (
            <Button
              size="xl"
              view="outlined"
              disabled={query.isFetching}
              onClick={() => void query.fetchNextPage({ cancelRefetch: false })}
            >
              Загрузить ещё выполнения
            </Button>
          )}
        </>
      )}
    </Card>
  )
}
