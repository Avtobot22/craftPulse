import { Button } from '@gravity-ui/uikit'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useId, useLayoutEffect, useRef, useState } from 'react'
import { findResolvedIngestConflict, resolveIngestConflict } from '@/shared/api'
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
  const [alreadyResolved, setAlreadyResolved] = useState(false)
  const field = useRef<HTMLSelectElement>(null)
  const id = useId()

  const actual = useQuery({
    queryKey: ['ingest-conflicts', 'resolution', conflict.id],
    queryFn: async ({ signal }) => {
      const resolved = await findResolvedIngestConflict(
        api,
        conflict.id,
        AbortSignal.any([signal, AbortSignal.timeout(30_000)]),
      )
      if (!resolved) throw new Error('Решение конфликта не найдено')
      return resolved
    },
    enabled: alreadyResolved,
    staleTime: Infinity,
  })
  const row = saved ?? actual.data ?? conflict
  const choice = selection ? resolutions[selection] : null
  const canEdit = canResolve && row.status === 'open' && !alreadyResolved

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
      <td>
        {contractLabel(statusLabels, alreadyResolved ? 'resolved' : row.status)}
      </td>
      <td>
        {row.resolution && (
          <span role={saved || actual.data ? 'status' : undefined}>
            {isResolution(row.resolution)
              ? resolutions[row.resolution].label
              : 'Неизвестное решение'}
          </span>
        )}
        {alreadyResolved && (
          <div className="cp-ingest-conflicts__editor">
            <span role="status">
              {actual.isFetching
                ? 'Конфликт уже решён. Загружаем решение.'
                : 'Конфликт уже решён другим администратором.'}
            </span>
            {actual.isError && (
              <>
                <span role="alert">Не удалось загрузить решение.</span>
                <Button size="xl" onClick={() => void actual.refetch()}>
                  Перечитать решение
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
            recheck={async (_attempt, _reason, signal) => {
              const result = await findResolvedIngestConflict(
                api,
                conflict.id,
                signal,
              )
              return result ? { kind: 'applied', result } : { kind: 'unknown' }
            }}
            onSaved={(result) => {
              setSaved(result)
              markListsStale()
            }}
            onRejected={(error) => {
              if (
                error.problem?.status === 409 &&
                error.problem.code === 'state_conflict'
              ) {
                setAlreadyResolved(true)
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
