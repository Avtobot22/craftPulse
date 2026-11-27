import { Text } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import {
  Link,
  Navigate,
  useNavigate,
  useOutletContext,
  useParams,
  useSearchParams,
} from 'react-router'
import {
  ApiError,
  listDocumentationRoots,
  listPersons,
  repositoryAreas,
} from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { canManageDocumentation } from '@/shared/auth'
import type { CurrentUser } from '@/shared/auth'
import {
  documentationCorpus,
  readTechnicalId,
  routePaths,
} from '@/shared/config'
import { ErrorState, LoadingState, UnavailableRedirect } from '@/shared/ui'
import { audienceLabel, noOwner } from '../model/root-set'
import { RootEditor } from './root-editor'
import './documentation-roots.css'

export function DocumentationRootsPage({ api }: { api: ApiClient }) {
  const { serviceId } = useParams()
  const id = readTechnicalId(serviceId)
  if (!id) return <Navigate to={routePaths.unavailable} replace />
  return <ServiceRoots key={id} api={api} serviceId={id} />
}

function ServiceRoots({
  api,
  serviceId,
}: {
  api: ApiClient
  serviceId: string
}) {
  const current = useOutletContext<CurrentUser>()
  const [search] = useSearchParams()
  const navigate = useNavigate()
  const query = useQuery({
    ...listDocumentationRoots(api, serviceId),
    refetchOnMount: 'always',
  })
  const repositories = useQuery({
    ...repositoryAreas(api),
    enabled: query.isSuccess && current.actions.includes('org.manage'),
  })
  const persons = useQuery({
    ...listPersons(api, { kind: 'human', limit: 100 }),
    enabled: query.isSuccess && current.actions.includes('accounts.manage'),
  })
  const owners = new Map(
    persons.isSuccess
      ? persons.data.data.map(({ id, display_name }) => [
          id,
          display_name ?? id,
        ])
      : [],
  )
  if (current.user.person_id && current.user.display_name) {
    owners.set(current.user.person_id, current.user.display_name)
  }
  const corpus = documentationCorpus(search, serviceId)
  const serviceName =
    current.areas.services.find(({ id }) => id === serviceId)?.name ?? serviceId

  if (
    query.error instanceof ApiError &&
    ['forbidden', 'not-found'].includes(query.error.kind)
  ) {
    return <UnavailableRedirect error={query.error} />
  }

  const rootSet = query.data?.data
  const loading =
    query.isPending || (query.isFetching && !query.isFetchedAfterMount)
  return (
    <article className="cp-doc-roots">
      <Text as="h1" variant="header-1">
        Корни документации
      </Text>
      <p>{serviceName}</p>
      <Link to={corpus}>К документам сервиса</Link>
      {loading && <LoadingState />}
      {query.isError && (
        <ErrorState
          reason="Не удалось загрузить корни документации"
          onRetry={() => void query.refetch()}
          retrying={query.isFetching}
        />
      )}
      {!loading &&
        query.data &&
        (canManageDocumentation(current, serviceId) ? (
          <RootEditor
            api={api}
            serviceId={serviceId}
            serviceName={serviceName}
            initial={query.data}
            repositories={
              repositories.isSuccess ? repositories.data : undefined
            }
            owners={owners}
            onSaved={() => void navigate(corpus)}
          />
        ) : (
          query.isSuccess &&
          (rootSet?.data.length ? (
            <ul className="cp-doc-roots__list">
              {rootSet.data.map((root) => (
                <li key={root.id}>
                  <dl>
                    <dt>Репозиторий</dt>
                    <dd>
                      {repositories.data?.get(root.area_id) ?? root.area_id}
                    </dd>
                    <dt>Путь</dt>
                    <dd>{root.path_prefix || 'Весь репозиторий'}</dd>
                    <dt>Профиль</dt>
                    <dd>{root.profile_id}</dd>
                    <dt>Владелец</dt>
                    <dd>
                      {root.owner_person_id
                        ? (owners.get(root.owner_person_id) ??
                          root.owner_person_id)
                        : noOwner}
                    </dd>
                    <dt>Аудитория</dt>
                    <dd>{audienceLabel(root.audience)}</dd>
                  </dl>
                </li>
              ))}
            </ul>
          ) : (
            <p>Корни документации не подключены.</p>
          ))
        ))}
    </article>
  )
}
