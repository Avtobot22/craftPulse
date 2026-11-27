import { Button } from '@gravity-ui/uikit'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useId, useLayoutEffect, useRef, useState } from 'react'
import { findIngestConflict, resolveIngestConflict } from '@/shared/api'
import type { ApiClient, components } from '@/shared/api'
import { contractLabel } from '@/shared/config'
import { ChangeConfirmationDialog } from '@/shared/ui'
import {
  isResolution,
  resolutionOptions,
  resolutions,
} from '../model/conflict-resolution'
import type { IngestConflict } from '../model/conflict-resolution'

type Input = components['schemas']['IngestConflictResolutionInput']

const statusLabels = { open: 'Открыт', resolved: 'Решён' }

export function ConflictResolution({
  api,
  conflict,
  canResolve,
}: {
  api: ApiClient
  conflict: IngestConflict
  canResolve: boolean
}) {
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState(false)
  const [selection, setSelection] = useState<Input['resolution'] | ''>('')
  const [proposal, setProposal] = useState<Input | null>(null)
  const [invalid, setInvalid] = useState(false)
  const [saved, setSaved] = useState<IngestConflict | null>(null)
  const [rejected, setRejected] = useState(false)
  const field = useRef<HTMLSelectElement>(null)
  const id = useId()

  const actual = useQuery({
    queryKey: ['ingest-conflicts', 'resolution', conflict.id],
    queryFn: async ({ signal }) => {
      const deadline = AbortSignal.any([signal, AbortSignal.timeout(30_000)])
      const current =
        (await findIngestConflict(api, conflict.id, 'resolved', deadline)) ??
        (await findIngestConflict(api, conflict.id, 'open', deadline))
      if (!current) throw new Error('Конфликт не найден')
      return current
    },
    enabled: false,
  })
  const checked =
    rejected && actual.isSuccess && actual.fetchStatus === 'idle'
      ? actual.data
      : undefined
  const row = saved ?? checked ?? conflict
  const choice = selection ? resolutions[selection] : null
  const canEdit =
    canResolve &&
    row.status === 'open' &&
    (!rejected || checked?.status === 'open')

  useLayoutEffect(() => {
    if (editing) field.current?.focus()
  }, [editing])

  function markListsStale() {
    // Preserve the current open page and refresh it when the user returns.
    void queryClient.invalidateQueries({
      queryKey: ['ingest-conflicts', 'list'],
      refetchType: 'none',
    })
    void queryClient.invalidateQueries({ queryKey: ['receipts'] })
  }

  return (
    <>
      <td>{contractLabel(statusLabels, row.status)}</td>
      <td>
        {row.resolution && (
          <span role={saved || checked ? 'status' : undefined}>
            {isResolution(row.resolution)
              ? resolutions[row.resolution].label
              : 'Неизвестное решение'}
          </span>
        )}
        {rejected && (
          <div className="cp-ingest-conflicts__editor">
            {checked?.status === 'resolved' && (
              <span role="status">
                Конфликт уже решён другим администратором.
              </span>
            )}
            {checked?.status === 'open' && (
              <span role="status">
                Конфликт остаётся открытым. Выберите другой вариант.
              </span>
            )}
            {actual.fetchStatus !== 'idle' && (
              <span role="status">Проверяем состояние конфликта.</span>
            )}
            {actual.fetchStatus === 'idle' && actual.isError && (
              <>
                <span role="alert">
                  Не удалось проверить состояние конфликта.
                </span>
                <Button size="xl" onClick={() => void actual.refetch()}>
                  Перечитать конфликт
                </Button>
              </>
            )}
          </div>
        )}
        {canEdit && (
          <>
            <Button
              size="xl"
              aria-label={'Решить конфликт ' + conflict.native_key}
              aria-expanded={editing}
              onClick={() => {
                setInvalid(false)
                setEditing(!editing)
              }}
            >
              Решить конфликт
            </Button>
            {editing && (
              <div className="cp-ingest-conflicts__editor">
                <label htmlFor={id}>Решение конфликта</label>
                <select
                  id={id}
                  ref={field}
                  value={selection}
                  aria-invalid={invalid || undefined}
                  aria-describedby={invalid ? id + '-error' : undefined}
                  onChange={(event) => {
                    const { value } = event.target
                    setSelection(isResolution(value) ? value : '')
                    setInvalid(false)
                  }}
                >
                  <option value="">Выберите решение</option>
                  {resolutionOptions.map((option) => (
                    <option key={option} value={option}>
                      {resolutions[option].label}
                    </option>
                  ))}
                </select>
                {invalid && (
                  <span id={id + '-error'} role="alert">
                    Не удалось применить выбранное решение. Проверьте выбор.
                  </span>
                )}
                <Button
                  size="xl"
                  view="action"
                  disabled={!choice}
                  onClick={() => {
                    if (!selection) return
                    setInvalid(false)
                    setProposal({ resolution: selection })
                  }}
                >
                  Применить решение конфликта
                </Button>
              </div>
            )}
          </>
        )}
        {canEdit && proposal && (
          <ChangeConfirmationDialog
            title="Подтверждение решения конфликта"
            scope={conflict.native_key}
            consequences={resolutions[proposal.resolution].consequence}
            proposal={{
              etag: '',
              draft: proposal,
              changes: [
                {
                  label: 'Решение конфликта',
                  before: 'Открыт',
                  after: resolutions[proposal.resolution].label,
                },
              ],
            }}
            initialWrite
            submit={({ draft }, signal) =>
              resolveIngestConflict(api, conflict.id, draft, signal)
            }
            recheck={async ({ draft }, _reason, signal) => {
              const result = await findIngestConflict(
                api,
                conflict.id,
                'resolved',
                signal,
              )
              if (!result?.resolution) return { kind: 'unknown' }
              if (result.resolution === draft.resolution)
                return { kind: 'applied', result }
              return {
                kind: 'different-result',
                result,
                title: 'Сохранено другое решение',
                message: isResolution(result.resolution)
                  ? resolutions[result.resolution].label
                  : 'Неизвестное решение',
              }
            }}
            onSaved={(result) => {
              setSaved(result)
              setRejected(false)
              markListsStale()
            }}
            onRejected={(error) => {
              if (
                error.problem?.status === 409 &&
                error.problem.code === 'state_conflict'
              ) {
                setRejected(true)
                setSelection('')
                void actual.refetch()
                markListsStale()
                return true
              }
              return false
            }}
            onValidation={() => {
              setInvalid(true)
              requestAnimationFrame(() => field.current?.focus())
            }}
            onClose={() => {
              setProposal(null)
            }}
          />
        )}
      </td>
    </>
  )
}
