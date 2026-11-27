import { Text } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { generatePath, Link, useParams, useSearchParams } from 'react-router'
import { exceptionOrigin } from '@/entities/exception'
import { getException } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { routePaths } from '@/shared/config'
import { ErrorState, InspectionBack, LoadingState } from '@/shared/ui'
import { ExceptionList } from './exception-list'
import { ExceptionHistory } from './exception-history'
import './exceptions.css'

export function ExceptionsPage({ api }: { api: ApiClient }) {
  const { exceptionId } = useParams()
  return (
    <article className="cp-exceptions">
      <Text as="h1" variant="header-1">
        История исключений
      </Text>
      {exceptionId ? (
        <ExceptionCard
          key={exceptionId}
          api={api}
          id={exceptionId.toLowerCase()}
        />
      ) : (
        <ExceptionList api={api} />
      )}
    </article>
  )
}

function ExceptionCard({ api, id }: { api: ApiClient; id: string }) {
  const [search] = useSearchParams()
  const query = useQuery({ ...getException(api, id), refetchOnMount: 'always' })
  const waiting =
    query.isPending || (query.isFetching && !query.isFetchedAfterMount)
  const origin = exceptionOrigin(
    search,
    query.data?.data.current_version.scope.finding_id ??
      search.get('finding_id'),
  )

  return (
    <>
      <nav className="cp-exceptions__actions" aria-label="Возврат">
        <InspectionBack
          fallbackTo={{
            pathname: generatePath(routePaths.exceptions),
            search: search.toString(),
          }}
        />
        {origin && <Link to={origin}>К исходной находке</Link>}
      </nav>
      {waiting ? (
        <LoadingState />
      ) : query.isError ? (
        <ErrorState
          reason="Не удалось загрузить решение"
          onRetry={() => void query.refetch()}
          retrying={query.isFetching}
        />
      ) : (
        <ExceptionHistory
          api={api}
          resource={query.data}
          refreshing={query.isFetching}
          asOf={query.dataUpdatedAt}
        />
      )}
    </>
  )
}
