import { Button } from '@gravity-ui/uikit'
import { useInfiniteQuery } from '@tanstack/react-query'
import { formatFindingLocation } from '@/entities/finding'
import { RestrictedResults } from '@/entities/result-access'
import { ApiError, findingPages } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import {
  contractLabel,
  documentationFinding,
  technicalLabels,
} from '@/shared/config'
import {
  ErrorState,
  InspectionLink,
  LoadingState,
  useInspectionReturn,
} from '@/shared/ui'

interface ViolationsScope {
  api: ApiClient
  serviceId: string
  search: URLSearchParams
}

export function DocumentationViolations(props: ViolationsScope) {
  return (
    <section aria-labelledby="documentation-violations">
      <h2 id="documentation-violations">Нарушения документации</h2>
      <ViolationList {...props} kind="docs" title="Документация" />
      <ViolationList {...props} kind="openapi" title="OpenAPI" />
    </section>
  )
}

function ViolationList({
  api,
  serviceId,
  search,
  kind,
  title,
}: ViolationsScope & {
  kind: 'docs' | 'openapi'
  title: string
}) {
  const query = useInfiniteQuery({
    ...findingPages(api, { service_id: serviceId, finding_kind: kind }),
    refetchOnMount: 'always',
  })
  const waiting =
    query.isPending || (query.isFetching && !query.isFetchedAfterMount)
  const unavailable =
    query.error instanceof ApiError &&
    ['forbidden', 'not-found'].includes(query.error.kind)
  const pages =
    !waiting && !unavailable && (query.isSuccess || query.isFetchNextPageError)
      ? query.data.pages
      : []
  const findings = [
    ...new Map(
      pages.flatMap(({ data }) =>
        data
          .filter((finding) => finding.finding_kind === kind)
          .map((finding) => [finding.id, finding] as const),
      ),
    ).values(),
  ]
  useInspectionReturn(pages.length > 0 && findings.length > 0)
  const limited = pages.some(
    ({ access_limited, access_notes }) =>
      access_limited || access_notes.length > 0,
  )
  const loadMore = () => void query.fetchNextPage({ cancelRefetch: false })

  return (
    <section aria-labelledby={`documentation-findings-${kind}`}>
      <h3 id={`documentation-findings-${kind}`}>{title}</h3>
      {waiting && <LoadingState />}
      {query.isError && (
        <ErrorState
          reason={
            query.isFetchNextPageError
              ? 'Не удалось загрузить следующие нарушения'
              : 'Не удалось загрузить нарушения'
          }
          onRetry={
            query.isFetchNextPageError ? loadMore : () => void query.refetch()
          }
          retrying={query.isFetching}
        />
      )}
      <RestrictedResults access_limited={limited} />
      {findings.length > 0 && (
        <ul>
          {findings.map((finding) => (
            <li key={finding.id}>
              <InspectionLink
                id={`documentation-violation-${finding.id}`}
                to={documentationFinding(search, serviceId, finding.id)}
              >
                {finding.rule_id} ·{' '}
                {finding.location
                  ? formatFindingLocation(finding.location)
                  : 'Место неизвестно'}
              </InspectionLink>
              {' · '}
              {contractLabel(technicalLabels.findingState, finding.state)}
            </li>
          ))}
        </ul>
      )}
      {pages.length > 0 && !findings.length && !query.hasNextPage && (
        <p>Нет доступных нарушений.</p>
      )}
      {query.hasNextPage && !query.isFetchNextPageError && (
        <Button size="l" disabled={query.isFetching} onClick={loadMore}>
          Ещё нарушения {title}
        </Button>
      )}
    </section>
  )
}
