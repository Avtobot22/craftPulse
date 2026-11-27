import { Button } from '@gravity-ui/uikit'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import {
  ApiError,
  findingPages,
  getDocumentationCheck,
  getFinding,
  listDocumentationRoots,
  requestDocumentationCheck,
  serviceDocumentPages,
} from '@/shared/api'
import type { ApiClient, DocumentationRoot, FindingDetail } from '@/shared/api'
import { ErrorState, LoadingState } from '@/shared/ui'

function containsFinding(root: DocumentationRoot, finding: FindingDetail) {
  const path = finding.location?.path
  return (
    root.area_id === finding.area_id &&
    (!root.path_prefix ||
      (path?.startsWith(root.path_prefix + '/') ?? false) ||
      path === root.path_prefix)
  )
}

function checkFailure(code: string | undefined) {
  switch (code) {
    case 'access_lost':
      return 'Доступ к репозиторию потерян.'
    case 'source_unavailable':
      return 'Источник недоступен.'
    case 'calculation_failed':
      return 'Не удалось обработать отчёт проверки.'
    case undefined:
    default:
      return 'Не удалось завершить повторную проверку.'
  }
}

export function DocumentationRecheck({
  api,
  serviceId,
  finding,
}: {
  api: ApiClient
  serviceId: string
  finding: FindingDetail
}) {
  const roots = useQuery({
    ...listDocumentationRoots(api, serviceId),
    select: ({ data }) => data.data,
    refetchOnMount: 'always',
  })
  const cache = useQueryClient()
  const [rootChoice, setRootChoice] = useState('')
  const [operationId, setOperationId] = useState<string>()
  const check = useMutation({
    mutationFn: (rootId: string) =>
      requestDocumentationCheck(api, serviceId, rootId),
    onSuccess: (operation) => {
      cache.setQueryData(
        getDocumentationCheck(api, operation.id).queryKey,
        operation,
      )
      setOperationId(operation.id)
    },
  })
  const operation = useQuery({
    ...getDocumentationCheck(api, operationId ?? ''),
    enabled: operationId !== undefined,
  })
  const status = operation.data?.status
  const active = status === 'queued' || status === 'running'
  const loading =
    roots.isPending || (roots.isFetching && !roots.isFetchedAfterMount)
  const candidates =
    !loading && roots.isSuccess
      ? roots.data.filter((root) => containsFinding(root, finding))
      : []
  const selected =
    candidates.find(({ id }) => id === rootChoice) ??
    (candidates.length === 1 ? candidates[0] : undefined)
  const findingId = finding.id

  const accessLost =
    status === 'failed' && operation.data?.error?.code === 'access_lost'

  useEffect(() => {
    if (status !== 'succeeded' && !accessLost) return
    void cache.invalidateQueries({
      queryKey: getFinding(api, findingId).queryKey,
    })
    void cache.invalidateQueries({
      queryKey: serviceDocumentPages(api, serviceId).queryKey,
    })
    for (const finding_kind of ['docs', 'openapi'] as const) {
      void cache.invalidateQueries({
        queryKey: findingPages(api, { service_id: serviceId, finding_kind })
          .queryKey,
      })
    }
  }, [accessLost, api, cache, findingId, operationId, serviceId, status])

  return (
    <section aria-labelledby="documentation-check-heading">
      <h2 id="documentation-check-heading">Повторная проверка</h2>
      {loading && <LoadingState />}
      {roots.isError && (
        <ErrorState
          reason="Не удалось получить корни сервиса"
          onRetry={() => void roots.refetch()}
          retrying={roots.isFetching}
        />
      )}
      {!loading && roots.isSuccess && !candidates.length && (
        <p>Подходящий Git-корень сервиса не найден.</p>
      )}
      {candidates.length > 0 && (
        <form
          onSubmit={(event) => {
            event.preventDefault()
            if (!selected || check.isPending || active) return
            setOperationId(undefined)
            check.mutate(selected.id)
          }}
        >
          {candidates.length > 1 ? (
            <label>
              Git-корень
              <select
                value={selected?.id ?? ''}
                disabled={active || check.isPending}
                onChange={(event) => {
                  setRootChoice(event.target.value)
                }}
              >
                <option value="">Выберите корень</option>
                {candidates.map(({ id, path_prefix, profile_id }) => (
                  <option key={id} value={id}>
                    {path_prefix || 'Весь репозиторий'} · {profile_id}
                  </option>
                ))}
              </select>
            </label>
          ) : (
            selected && (
              <p>Git-корень: {selected.path_prefix || 'Весь репозиторий'}</p>
            )
          )}
          <p>
            Для Git-корня результат придёт со следующим pipeline. Запрос не
            запускает CI.
          </p>
          <Button
            size="l"
            view="action"
            type="submit"
            disabled={!selected || active || check.isPending}
          >
            Проверить снова
          </Button>
        </form>
      )}
      {check.isError && (
        <p role="alert">
          {check.error instanceof ApiError &&
          check.error.problem?.status === 409
            ? 'Для сервиса уже выполняется проверка другого корня.'
            : 'Не удалось запросить повторную проверку.'}
        </p>
      )}
      {active && (
        <p role="status">Запрос принят. Ожидаем отчёт следующего pipeline.</p>
      )}
      {status === 'succeeded' && (
        <p role="status">
          Получен отчёт повторной проверки. Состояние нарушения обновляется по
          отчёту.
        </p>
      )}
      {status === 'failed' && (
        <p role="alert">{checkFailure(operation.data?.error?.code)}</p>
      )}
      {operation.isError && (
        <ErrorState
          reason="Не удалось узнать состояние повторной проверки"
          onRetry={() => void operation.refetch()}
          retrying={operation.isFetching}
        />
      )}
    </section>
  )
}
