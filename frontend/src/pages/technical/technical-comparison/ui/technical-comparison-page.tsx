import { Text } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { useParams, useSearchParams } from 'react-router'
import {
  compareTechnicalResults,
  getFinding,
  InvalidResponse,
} from '@/shared/api'
import type { ApiClient, TechnicalComparisonParams } from '@/shared/api'
import { technicalLabels } from '@/shared/config'
import {
  ErrorState,
  InspectionBack,
  LoadingState,
  useInspectionReturn,
} from '@/shared/ui'
import { ComparisonForm } from './comparison-form'
import { ComparisonResult } from './comparison-result'
import './technical-comparison-page.css'

const uuid = /^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/iu
const gitSha = /^(?:[0-9a-f]{40}|[0-9a-f]{64})$/u

function readParams(
  params: URLSearchParams,
  componentId: string,
): TechnicalComparisonParams | undefined {
  const base = params.get('base_sha')
  const head = params.get('head_sha')
  const kind = params.get('analysis_kind')
  if (
    !uuid.test(componentId) ||
    !base ||
    !head ||
    !gitSha.test(base) ||
    !gitSha.test(head) ||
    !isAnalysisKind(kind)
  )
    return undefined
  return {
    component_id: componentId.toLowerCase(),
    base_sha: base,
    head_sha: head,
    analysis_kind: kind,
  }
}

function isAnalysisKind(
  kind: string | null,
): kind is TechnicalComparisonParams['analysis_kind'] {
  return kind !== null && Object.hasOwn(technicalLabels.analysisKind, kind)
}

export function TechnicalComparisonPage({ api }: { api: ApiClient }) {
  const { componentId: routeComponent } = useParams()
  const [params] = useSearchParams()
  const findingId = params.get('finding_id') ?? ''
  const resolveComponent = routeComponent === undefined && uuid.test(findingId)
  const finding = useQuery({
    ...getFinding(api, findingId),
    enabled: resolveComponent,
    retry: false,
  })
  const findingComponent =
    finding.isSuccess && finding.data.id === findingId
      ? finding.data.component_id
      : null
  const componentId = routeComponent ?? findingComponent ?? ''
  const coordinates = readParams(params, componentId)

  return (
    <article className="cp-technical-comparison">
      <Text as="h1" variant="header-1">
        Сравнение результатов проверок
      </Text>
      <InspectionBack />
      {resolveComponent && finding.isPending ? (
        <LoadingState />
      ) : (
        <ComparisonForm
          key={JSON.stringify([params.toString(), componentId])}
          componentId={componentId}
          params={params}
        />
      )}
      {resolveComponent && !finding.isPending && !findingComponent && (
        <p>Не удалось определить компонент по находке. Укажите его ID.</p>
      )}
      {!params.get('base_sha') && (
        <p>Сопоставимой базы нет. Для сравнения укажите SHA базы.</p>
      )}
      {coordinates && <ComparisonContent api={api} coordinates={coordinates} />}
    </article>
  )
}

function ComparisonContent({
  api,
  coordinates,
}: {
  api: ApiClient
  coordinates: TechnicalComparisonParams
}) {
  const query = useQuery({
    ...compareTechnicalResults(api, coordinates),
    retry: false,
    select: (result) => {
      if (
        result.base.sha !== coordinates.base_sha ||
        result.head.sha !== coordinates.head_sha
      )
        throw new InvalidResponse()
      return result
    },
  })
  useInspectionReturn(query.isSuccess)
  if (query.isPending) return <LoadingState />
  if (query.isError)
    return (
      <ErrorState
        reason="Не удалось загрузить сравнение"
        retrying={query.isFetching}
        onRetry={() => void query.refetch()}
      />
    )
  return <ComparisonResult comparison={query.data} />
}
