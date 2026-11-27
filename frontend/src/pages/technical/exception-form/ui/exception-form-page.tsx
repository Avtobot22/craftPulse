import { Alert, Text } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { generatePath, useParams, useSearchParams } from 'react-router'
import { exceptionOrigin } from '@/entities/exception'
import {
  ApiError,
  getException,
  getFinding,
  isStrongETag,
  isUnavailableError,
} from '@/shared/api'
import type { ApiClient, ExceptionScope } from '@/shared/api'
import { routePaths } from '@/shared/config'
import {
  ErrorState,
  InspectionBack,
  LoadingState,
  UnavailableRedirect,
} from '@/shared/ui'
import { scopeOptions } from '../model/exception-draft'
import { ExceptionEditor } from './exception-editor'
import './exception-form.css'

export function ExceptionFormPage({ api }: { api: ApiClient }) {
  const params = useParams()
  const findingId = params.findingId?.toLowerCase()
  const exceptionId = params.exceptionId?.toLowerCase()
  const [search] = useSearchParams()
  const requestedFinding = exceptionOrigin(search, findingId)
  const existing = useQuery({
    ...getException(api, exceptionId ?? ''),
    enabled: Boolean(exceptionId),
    refetchOnMount: 'always',
    refetchOnWindowFocus: false,
  })
  const version = existing.data?.data.current_version
  const sourceId = findingId ?? version?.scope.finding_id
  const finding = useQuery({
    ...getFinding(api, sourceId ?? ''),
    enabled: Boolean(sourceId),
  })
  const useCachedFinding =
    !exceptionId &&
    finding.isRefetchError &&
    finding.error instanceof ApiError &&
    finding.error.kind === 'network'
  const source =
    (finding.isSuccess || useCachedFinding) && finding.data.id === sourceId
      ? finding.data
      : undefined
  const sourceArea = source?.area_id
  const anchor: ExceptionScope | undefined =
    source && sourceArea && (!version || sourceArea === version.scope.area_id)
      ? {
          area_id: sourceArea,
          finding_id: source.id,
          component_id: source.component_id,
          rule_id: source.rule_id || null,
          path_prefix: source.location?.path ?? null,
          package_purl: source.package?.purl ?? null,
        }
      : version?.scope

  const fallback = exceptionId
    ? {
        pathname: generatePath(routePaths.exceptions, { exceptionId }),
        search: search.toString(),
      }
    : (requestedFinding ?? routePaths.findings)
  const query = exceptionId ? existing : finding
  const waiting =
    query.isPending ||
    (query.isFetching && !query.isFetchedAfterMount) ||
    Boolean(sourceId && finding.isPending)
  const canEdit = exceptionId
    ? !existing.isError && isStrongETag(existing.data?.etag)
    : source?.actions.includes('create_exception')

  if (isUnavailableError(query.error))
    return <UnavailableRedirect error={query.error} />

  return (
    <article className="cp-exception-form">
      <Text as="h1" variant="header-1">
        {exceptionId ? 'Продлить исключение' : 'Оформить исключение'}
      </Text>
      <InspectionBack fallbackTo={fallback} />
      {!waiting && query.isError && (
        <ErrorState
          reason={
            useCachedFinding
              ? 'Не удалось обновить карточку находки.'
              : 'Не удалось загрузить данные исключения.'
          }
          onRetry={() => void query.refetch()}
          retrying={query.isFetching}
        />
      )}
      {waiting ? (
        <LoadingState />
      ) : canEdit &&
        anchor &&
        !version?.external_authority &&
        version?.decision_type !== 'bypass' ? (
        <ExceptionEditor
          key={exceptionId ?? findingId}
          api={api}
          scopes={scopeOptions(anchor, version?.scope)}
          existing={existing.data}
          findingId={findingId ?? anchor.finding_id}
        />
      ) : !query.isError ? (
        <Alert
          theme="warning"
          message={
            version?.external_authority
              ? 'Решение изменяется во внешнем анализаторе.'
              : source && !source.actions.includes('create_exception')
                ? 'Исключение может оформить техлид или разработчик с правом на эту область.'
                : 'Данные для оформления исключения недоступны.'
          }
        />
      ) : null}
    </article>
  )
}
