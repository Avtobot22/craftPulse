import { useQuery } from '@tanstack/react-query'
import { Text } from '@gravity-ui/uikit'
import type { ReactNode } from 'react'
import { useParams } from 'react-router'
import { ApiError, readArray, readRecord, readString } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { ErrorState, LoadingState } from '@/shared/ui'

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
