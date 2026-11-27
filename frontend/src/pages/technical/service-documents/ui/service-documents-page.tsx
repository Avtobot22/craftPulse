import { Text } from '@gravity-ui/uikit'
import { useInfiniteQuery } from '@tanstack/react-query'
import {
  Link,
  Navigate,
  useOutletContext,
  useParams,
  useSearchParams,
} from 'react-router'
import { RestrictedResults } from '@/entities/result-access'
import { ApiError, serviceDocumentPages } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { canManageDocumentation } from '@/shared/auth'
import type { CurrentUser } from '@/shared/auth'
import {
  documentationCorpus,
  documentationRoots,
  readTechnicalId,
  routePaths,
} from '@/shared/config'
import {
  DataStateBadge,
  ErrorState,
  LoadingState,
  UnavailableRedirect,
} from '@/shared/ui'
import type { DataState } from '@/shared/ui'
import { DocumentList } from './document-list'
import { DocumentationViolations } from './documentation-violations'
import './service-documents.css'

function reviewState(state: string | undefined): DataState {
  switch (state) {
    case 'ASSESSED':
    case 'PROVISIONAL':
    case 'NOT_EVALUATED':
    case 'NOT_APPLICABLE':
    case 'BOUNDED':
      return state
    case undefined:
    default:
      return 'UNKNOWN'
  }
}

export function ServiceDocumentsPage({ api }: { api: ApiClient }) {
  const { serviceId } = useParams()
  const id = readTechnicalId(serviceId)
  const current = useOutletContext<CurrentUser>()
  const [search] = useSearchParams()

  if (serviceId && !id) return <Navigate to={routePaths.unavailable} replace />

  return (
    <article className="cp-service-docs">
      <Text as="h1" variant="header-1">
        Документация сервиса
      </Text>
      {id ? (
        <ServiceCorpus
          key={id}
          api={api}
          serviceId={id}
          current={current}
          search={search}
        />
      ) : (
        <section aria-label="Выбор сервиса">
          {current.areas.services.length ? (
            <ul>
              {current.areas.services.map(({ id, name }) => (
                <li key={id}>
                  <Link to={documentationCorpus(search, id)}>{name}</Link>
                </li>
              ))}
            </ul>
          ) : (
            <p>Нет доступных сервисов.</p>
          )}
        </section>
      )}
    </article>
  )
}

function ServiceCorpus({
  api,
  serviceId,
  current,
  search,
}: {
  api: ApiClient
  serviceId: string
  current: CurrentUser
  search: URLSearchParams
}) {
  const query = useInfiniteQuery({
    ...serviceDocumentPages(api, serviceId),
    refetchOnMount: 'always',
  })
  const waiting =
    query.isPending || (query.isFetching && !query.isFetchedAfterMount)
  const pages =
    !waiting && (query.isSuccess || query.isFetchNextPageError)
      ? query.data.pages
      : []
  const state = reviewState(pages[0]?.documentation_state)
  const limited = pages.some(
    (page) => page.access_limited || page.access_notes.length > 0,
  )
  const documents = [
    ...new Map(
      pages.flatMap(({ data }) =>
        data.map((document) => [document.id, document] as const),
      ),
    ).values(),
  ]
  const disconnected =
    pages[0]?.documentation_state === 'UNKNOWN' &&
    !limited &&
    documents.length === 0 &&
    !query.hasNextPage

  if (
    query.error instanceof ApiError &&
    ['forbidden', 'not-found'].includes(query.error.kind)
  ) {
    return <UnavailableRedirect error={query.error} />
  }

  const loadMore = () => void query.fetchNextPage({ cancelRefetch: false })
  return (
    <>
      <nav aria-label="Документация сервиса">
        <Link to={documentationCorpus(search)}>Все сервисы</Link>
        {canManageDocumentation(current, serviceId) && (
          <Link to={documentationRoots(search, serviceId)}>
            Настроить корни документации
          </Link>
        )}
      </nav>
      <p>
        {current.areas.services.find(({ id }) => id === serviceId)?.name ??
          serviceId}
      </p>
      {waiting && <LoadingState />}
      {query.isError && (
        <ErrorState
          reason={
            query.isFetchNextPageError
              ? 'Не удалось загрузить следующие документы'
              : 'Не удалось загрузить документы'
          }
          retrying={query.isFetching}
          onRetry={
            query.isFetchNextPageError ? loadMore : () => void query.refetch()
          }
        />
      )}
      {pages.length > 0 && (
        <>
          <div>
            <DataStateBadge state={state} />
            {disconnected && <p>Документация не подключена.</p>}
            <RestrictedResults access_limited={limited} />
          </div>
          {!disconnected && (
            <>
              <DocumentList
                documents={documents}
                hasMore={query.hasNextPage}
                loading={query.isFetching}
                loadMore={loadMore}
              />
              {current.actions.includes('findings.read') && (
                <DocumentationViolations
                  api={api}
                  serviceId={serviceId}
                  search={search}
                />
              )}
            </>
          )}
        </>
      )}
    </>
  )
}
