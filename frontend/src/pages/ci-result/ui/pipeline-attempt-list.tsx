import { Button } from '@gravity-ui/uikit'
import type { ReactNode } from 'react'
import { ErrorState, LoadingState } from '@/shared/ui'
import { isAccessError } from '../model/pipeline'
import type { Attempt } from '../model/pipeline'
import type { usePipelineAttempts } from '../model/use-pipeline-attempts'

export function PipelineAttemptList({
  query,
  children,
}: {
  query: ReturnType<typeof usePipelineAttempts>
  children: (data: { attempts: Attempt[]; hasNextPage: boolean }) => ReactNode
}) {
  if (query.isPending) return <LoadingState />
  if (isAccessError(query.error)) return <p>Попытки CI недоступны.</p>
  const pages = query.data?.pages ?? []
  const firstPage = pages[0]
  return (
    <>
      {firstPage &&
        children({
          attempts: pages.flatMap(({ data }) => data),
          hasNextPage: query.hasNextPage,
        })}
      {query.isError && (
        <ErrorState
          reason="Не удалось загрузить попытки CI"
          retrying={query.isFetching}
          onRetry={() => {
            if (query.isFetchNextPageError)
              void query.fetchNextPage({ cancelRefetch: false })
            else void query.refetch()
          }}
        />
      )}
      {query.hasNextPage && !query.isFetchNextPageError && (
        <Button
          size="xl"
          view="outlined"
          disabled={query.isFetching}
          onClick={() => void query.fetchNextPage({ cancelRefetch: false })}
        >
          Загрузить ещё попытки
        </Button>
      )}
    </>
  )
}
