import { Button, Text } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import {
  InspectionBack,
  ErrorState,
  LoadingState,
  UnavailableRedirect,
} from '@/shared/ui'
import {
  Navigate,
  useOutletContext,
  useParams,
  useSearchParams,
} from 'react-router'
import { ApiError, getFinding } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { canManageDocumentation } from '@/shared/auth'
import type { CurrentUser } from '@/shared/auth'
import {
  documentationCorpus,
  readTechnicalId,
  routePaths,
} from '@/shared/config'
import { DocumentationEvidence } from './documentation-evidence'
import { DocumentationRecheck } from './documentation-recheck'
import './documentation-finding.css'

export function DocumentationFindingPage({ api }: { api: ApiClient }) {
  const { findingId } = useParams()
  const id = readTechnicalId(findingId)
  const current = useOutletContext<CurrentUser>()
  if (!id || !current.actions.includes('docs.read')) {
    return <Navigate to={routePaths.unavailable} replace />
  }
  return <FindingView key={id} api={api} id={id} current={current} />
}

function FindingView({
  api,
  id,
  current,
}: {
  api: ApiClient
  id: string
  current: CurrentUser
}) {
  const [search] = useSearchParams()
  const serviceId = readTechnicalId(search.get('service_id'))
  const query = useQuery({ ...getFinding(api, id), refetchOnMount: 'always' })
  const waiting =
    query.isPending || (query.isFetching && !query.isFetchedAfterMount)
  const finding = !waiting && query.isSuccess ? query.data : undefined

  if (
    (query.error instanceof ApiError &&
      ['forbidden', 'not-found'].includes(query.error.kind)) ||
    (finding &&
      (finding.id !== id ||
        !['docs', 'openapi'].includes(finding.finding_kind)))
  ) {
    return (
      <UnavailableRedirect
        error={query.error instanceof ApiError ? query.error : undefined}
      />
    )
  }

  return (
    <article className="cp-doc-finding">
      <Text as="h1" variant="header-1">
        Нарушение документации
      </Text>
      <div>
        <InspectionBack
          label="К документам сервиса"
          fallbackTo={documentationCorpus(search, serviceId)}
        />
      </div>
      {waiting && <LoadingState />}
      {query.isError && (
        <ErrorState
          reason="Не удалось загрузить нарушение"
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      )}
      {finding && (
        <>
          <DocumentationEvidence finding={finding} />
          {serviceId && canManageDocumentation(current, serviceId) && (
            <DocumentationRecheck
              key={serviceId}
              api={api}
              serviceId={serviceId}
              finding={finding}
            />
          )}
          <div>
            <Button
              size="l"
              disabled={query.isFetching}
              onClick={() => void query.refetch()}
            >
              Обновить нарушение
            </Button>
          </div>
        </>
      )}
    </article>
  )
}
