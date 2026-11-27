import { Text } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { Navigate, useLocation, useParams } from 'react-router'
import { ApiError, getFinding, InvalidResponse } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { routePaths } from '@/shared/config'
import {
  ErrorState,
  InspectionBack,
  LoadingState,
  useInspectionReturn,
} from '@/shared/ui'
import { FindingDetails } from './finding-details'
import './finding-detail-page.css'

export function FindingDetailPage({ api }: { api: ApiClient }) {
  const { findingId = '' } = useParams()
  const state: unknown = useLocation().state
  const id = findingId.toLowerCase()
  const query = useQuery({
    ...getFinding(api, id),
    refetchOnMount: 'always',
    select: (finding) => {
      if (finding.id !== id) throw new InvalidResponse()
      return finding
    },
  })
  const waiting =
    query.isPending || (query.isFetching && !query.isFetchedAfterMount)
  const ready = query.isSuccess && !waiting
  useInspectionReturn(ready)

  if (
    query.error instanceof ApiError &&
    (query.error.kind === 'not-found' ||
      query.error.kind === 'forbidden' ||
      query.error.problem?.status === 404)
  ) {
    const fromInspection =
      state !== null &&
      typeof state === 'object' &&
      'fromInspection' in state &&
      state.fromInspection === true
    return (
      <Navigate
        to={routePaths.unavailable}
        replace
        state={{ fromInspection }}
      />
    )
  }

  return (
    <article className="cp-finding-detail">
      <Text as="h1" variant="header-1">
        Карточка находки
      </Text>
      <div>
        <InspectionBack fallbackTo={routePaths.findings} />
      </div>
      {waiting && <LoadingState />}
      {!waiting && query.isError && (
        <ErrorState
          reason="Не удалось загрузить карточку находки"
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      )}
      {ready && <FindingDetails api={api} finding={query.data} />}
    </article>
  )
}
