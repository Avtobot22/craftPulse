import { Button } from '@gravity-ui/uikit'
import { useInfiniteQuery } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import { Navigate } from 'react-router'
import { RestrictedResults } from '@/entities/result-access'
import { ApiError, findingPages } from '@/shared/api'
import type { ApiClient, FindingListParams } from '@/shared/api'
import { routePaths } from '@/shared/config'
import {
  ErrorState,
  LoadingState,
  useInspectionPageCount,
  useInspectionReturn,
} from '@/shared/ui'
import { FindingTable } from './finding-table'

export function FindingResults({
  api,
  params,
}: {
  api: ApiClient
  params: FindingListParams
}) {
  const query = useInfiniteQuery({
    ...findingPages(api, params),
    refetchOnMount: 'always',
  })
  const pageStart = useRef<HTMLDivElement>(null)
  const returnPageCount = useInspectionPageCount()
  const { fetchNextPage, isFetching } = query
  const pageCount = query.data?.pages.length ?? 0
  const restoringPages =
    query.hasNextPage && !query.isError && pageCount < returnPageCount
  useEffect(() => {
    if (restoringPages && !isFetching) {
      void fetchNextPage({ cancelRefetch: false })
    }
  }, [restoringPages, isFetching, fetchNextPage])

  const waiting =
    query.isPending ||
    (isFetching && !query.isFetchedAfterMount) ||
    restoringPages
  const pages =
    !waiting && (query.isSuccess || query.isFetchNextPageError)
      ? query.data.pages
      : undefined
  useInspectionReturn(pages !== undefined, 'findings-heading')

  if (
    query.error instanceof ApiError &&
    (query.error.kind === 'forbidden' || query.error.kind === 'not-found')
  ) {
    return <Navigate to={routePaths.unavailable} replace />
  }
  if (waiting) return <LoadingState />

  const findings = [
    ...new Map(
      pages?.flatMap(({ data }) =>
        data.map((finding) => [finding.id, finding] as const),
      ),
    ).values(),
  ]
  const limited =
    pages?.some(
      ({ access_limited, access_notes }) =>
        access_limited || access_notes.length > 0,
    ) ?? false
  const loadMore = () => void query.fetchNextPage({ cancelRefetch: false })

  return (
    <>
      <RestrictedResults access_limited={limited} />
      <div ref={pageStart} tabIndex={-1} aria-label="Результаты поиска">
        {findings.length > 0 && (
          <FindingTable findings={findings} pageCount={pageCount} />
        )}
        {pages && !findings.length && !query.hasNextPage && (
          <p>
            {limited
              ? 'Нет доступных находок по фильтру'
              : 'По фильтру находок нет'}
          </p>
        )}
      </div>
      {query.isError && (
        <ErrorState
          reason={
            query.isFetchNextPageError
              ? 'Не удалось загрузить следующие находки'
              : 'Не удалось загрузить находки'
          }
          retrying={query.isFetching}
          onRetry={
            query.isFetchNextPageError ? loadMore : () => void query.refetch()
          }
        />
      )}
      {query.hasNextPage && !query.isFetchNextPageError && (
        <div>
          <Button
            size="xl"
            disabled={query.isFetching}
            onClick={() => {
              pageStart.current?.focus()
              loadMore()
            }}
          >
            {query.isFetchingNextPage ? 'Загрузка' : 'Ещё'}
          </Button>
        </div>
      )}
    </>
  )
}
