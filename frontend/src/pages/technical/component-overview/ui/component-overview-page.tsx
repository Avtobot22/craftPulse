import { Button, Text } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import type { SubmitEvent } from 'react'
import {
  generatePath,
  useNavigate,
  useOutletContext,
  useParams,
  useSearchParams,
} from 'react-router'
import {
  isUnavailableError,
  getComponentOverview,
  InvalidResponse,
} from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import type { CurrentUser } from '@/shared/auth'
import {
  baselineContext,
  readTechnicalId,
  routePaths,
  technicalIdPattern,
} from '@/shared/config'
import {
  ErrorState,
  InspectionBack,
  LoadingState,
  UnavailableRedirect,
  useInspectionReturn,
} from '@/shared/ui'
import { OverviewDetails } from './overview-details'
import './component-overview.css'

export function ComponentOverviewPage({ api }: { api: ApiClient }) {
  const { componentId: routeId } = useParams()
  const componentId = readTechnicalId(routeId)
  const [search] = useSearchParams()
  const navigate = useNavigate()
  const current = useOutletContext<CurrentUser>()
  const query = useQuery({
    ...getComponentOverview(api, componentId ?? ''),
    enabled: componentId !== undefined,
    refetchOnMount: 'always',
    select: (overview) => {
      if (overview.component.id !== componentId) throw new InvalidResponse()
      return overview
    },
  })
  const waiting =
    componentId !== undefined &&
    (query.isPending || (query.isFetching && !query.isFetchedAfterMount))
  const ready = query.isSuccess && !waiting
  useInspectionReturn(ready, 'component-overview-heading')

  function openComponent(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    const input = new FormData(event.currentTarget).get('component_id')
    const id =
      typeof input === 'string' ? readTechnicalId(input.trim()) : undefined
    if (id)
      void navigate({
        pathname: generatePath(routePaths.componentOverview, {
          componentId: id,
        }),
        search: baselineContext(search).toString(),
      })
  }

  if (isUnavailableError(query.error))
    return <UnavailableRedirect error={query.error} />

  return (
    <article className="cp-component-overview">
      <Text
        as="h1"
        id="component-overview-heading"
        tabIndex={-1}
        variant="header-1"
      >
        Технический обзор компонента
      </Text>
      <div>
        <InspectionBack />
      </div>
      {!componentId && (
        <form
          className="cp-component-overview__selector"
          onSubmit={openComponent}
        >
          {routeId && (
            <p role="alert">Некорректный ID в ссылке на компонент.</p>
          )}
          <label>
            ID компонента
            <input name="component_id" required pattern={technicalIdPattern} />
          </label>
          <Button type="submit" size="xl" view="action">
            Открыть компонент
          </Button>
        </form>
      )}
      {waiting && <LoadingState />}
      {!waiting && query.isError && (
        <ErrorState
          reason="Не удалось загрузить обзор компонента"
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      )}
      {ready && (
        <OverviewDetails
          overview={query.data}
          search={search}
          current={current}
        />
      )}
    </article>
  )
}
