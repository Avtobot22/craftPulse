import { useQuery } from '@tanstack/react-query'
import { Text } from '@gravity-ui/uikit'
import type { ReactNode } from 'react'
import { Link, useParams, useSearchParams } from 'react-router'
import { ApiError, readArray, readRecord, readString } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { ErrorState, LoadingState } from '@/shared/ui'
import { readSummary, type Summary } from '../api/read-summary'
import { SummaryMetrics } from './summary-metrics'

function ResultPage({
  title,
  pending,
  error,
  retry,
  children,
}: {
  title: string
  pending: boolean
  error: Error | null
  retry: () => void
  children: ReactNode
}) {
  const unavailable =
    error instanceof ApiError &&
    (error.kind === 'not-found' || error.kind === 'forbidden')

  return (
    <section>
      <Text as="h1" variant="header-1">
        {title}
      </Text>
      {pending && <LoadingState />}
      {unavailable && <p>Результат недоступен или ещё не готов.</p>}
      {error && !unavailable && (
        <ErrorState
          reason="Не удалось загрузить результат CI"
          onRetry={retry}
        />
      )}
      {!error && children}
    </section>
  )
}

function readPipeline(value: unknown) {
  const pipeline = readRecord(value)
  return {
    ref: readString(pipeline.ref_name),
    sha: readString(pipeline.sha),
    outcome: readString(pipeline.outcome),
    reports: readArray(pipeline.reports, (value) => {
      const report = readRecord(value)
      return {
        kind: readString(report.analysis_kind),
        state: readString(report.state),
      }
    }),
  }
}

const publicationLabels: Record<Summary['publication']['status'], string> = {
  not_requested: 'Публикация комментария не запрошена',
  pending: 'Комментарий ожидает публикации',
  succeeded: 'Комментарий опубликован',
  failed: 'Не удалось опубликовать комментарий',
  unknown: 'Исход публикации комментария неизвестен',
  stale: 'Публикация комментария отменена',
}

const publicationReasons = new Map([
  ['write_permission_missing', 'нет права записи в MR'],
  ['head_changed', 'появился новый коммит'],
  ['provider_error', 'ошибка Git-хостинга'],
])

export function MergeRequestSummaryPage({ api }: { api: ApiClient }) {
  const { mergeRequestId = '' } = useParams()
  const [params] = useSearchParams()
  const head = params.get('head_sha') ?? undefined
  const revision = params.get('revision_no') ?? undefined
  const query = useQuery({
    queryKey: ['merge-request-summary', mergeRequestId, head, revision],
    queryFn: async ({ signal }) => {
      const response = await api.read(
        '/merge-requests/{merge_request_id}/summary',
        readSummary,
        {
          signal,
          pathParams: { merge_request_id: mergeRequestId },
          query: { head_sha: head, revision_no: revision },
        },
      )
      return response.data
    },
    retry: false,
  })
  const summary = query.data
  const errorCode = summary?.publication.errorCode
  const publicationReason =
    errorCode && (publicationReasons.get(errorCode) ?? errorCode)
  return (
    <ResultPage
      title="Сводка MR"
      error={query.error}
      pending={query.isPending}
      retry={() => void query.refetch()}
    >
      {summary && (
        <>
          <p>
            Коммит <code>{summary.head}</code>
          </p>
          {(summary.status === 'stale' || summary.status === 'superseded') && (
            <p>Историческая сводка</p>
          )}
          {summary.pipelineId && (
            <Link to={'/pipelines/' + encodeURIComponent(summary.pipelineId)}>
              Открыть pipeline
            </Link>
          )}
          <p>
            {publicationLabels[summary.publication.status]}
            {publicationReason && ` (${publicationReason})`}
          </p>
          <SummaryMetrics summary={summary} />
          {summary.limited && <p>Часть данных скрыта правами доступа.</p>}
          {summary.items.length > 0 && (
            <ul>
              {summary.items.map((item, index) => (
                <li key={index}>
                  {[item.analysis, item.kind, item.rule]
                    .filter(Boolean)
                    .join(' · ')}
                </li>
              ))}
            </ul>
          )}
        </>
      )}
    </ResultPage>
  )
}

export function PipelinePage({ api }: { api: ApiClient }) {
  const { pipelineId = '' } = useParams()
  const query = useQuery({
    queryKey: ['pipeline', pipelineId],
    queryFn: async ({ signal }) => {
      const response = await api.read(
        '/pipelines/{pipeline_id}',
        readPipeline,
        {
          signal,
          pathParams: { pipeline_id: pipelineId },
        },
      )
      return response.data
    },
    retry: false,
  })
  const pipeline = query.data
  return (
    <ResultPage
      title="Pipeline"
      error={query.error}
      pending={query.isPending}
      retry={() => void query.refetch()}
    >
      {pipeline && (
        <>
          <p>
            {pipeline.ref} · <code>{pipeline.sha}</code>
          </p>
          <p>Исход: {pipeline.outcome}</p>
          {pipeline.reports.length > 0 && (
            <ul>
              {pipeline.reports.map((report, index) => (
                <li key={index}>
                  {report.kind} · {report.state}
                </li>
              ))}
            </ul>
          )}
        </>
      )}
    </ResultPage>
  )
}
