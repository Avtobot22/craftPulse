import { Button, Flex } from '@gravity-ui/uikit'
import { useQueryClient } from '@tanstack/react-query'
import { useRef, useState } from 'react'
import { useNavigate } from 'react-router'
import {
  ApiError,
  getConnection,
  readConnectionById,
  setConnectionPaused,
  type ApiClient,
} from '@/shared/api'
import { routePaths } from '@/shared/config'
import { ChangeConfirmationDialog, type ChangeProposal } from '@/shared/ui'

type Resource = Awaited<ReturnType<typeof readConnectionById>>
type Draft = { kind: 'pause'; paused: boolean } | { kind: 'disconnect' }
type Result = { kind: 'pause' } | { kind: 'disconnect' }

function makeProposal(resource: Resource, draft: Draft): ChangeProposal<Draft> {
  const current = resource.data.sync_paused
  return {
    etag: resource.etag ?? '',
    draft,
    changes:
      draft.kind === 'disconnect'
        ? [{ label: 'Источник', before: 'Подключён', after: 'Отключён' }]
        : current === draft.paused
          ? []
          : [
              {
                label: 'Сбор',
                before: current ? 'Приостановлен' : 'Разрешён',
                after: draft.paused ? 'Приостановлен' : 'Разрешён',
              },
            ],
  }
}

export function ConnectionActions({
  api,
  resource,
  refresh,
  status,
}: {
  api: ApiClient
  resource: Resource
  refresh: () => Promise<void>
  status: string | undefined
}) {
  const client = useQueryClient()
  const navigate = useNavigate()
  const [proposal, setProposal] = useState<ChangeProposal<Draft> | null>(null)
  const [error, setError] = useState(false)
  const opener = useRef<HTMLElement | null>(null)
  const connection = resource.data
  const key = getConnection(api, connection.id).queryKey
  const paused = connection.sync_paused
  const disconnect = proposal?.draft.kind === 'disconnect'

  function open(draft: Draft, trigger: HTMLElement) {
    opener.current = trigger
    setError(false)
    setProposal(makeProposal(resource, draft))
  }

  return (
    <>
      <Flex wrap gap="spacing-3">
        {status === 'active' && (
          <Button
            onClick={(event) => {
              open({ kind: 'pause', paused: !paused }, event.currentTarget)
            }}
          >
            {paused ? 'Возобновить сбор' : 'Остановить загрузку'}
          </Button>
        )}
        <Button
          view="outlined-danger"
          onClick={(event) => {
            open({ kind: 'disconnect' }, event.currentTarget)
          }}
        >
          Отключить источник
        </Button>
      </Flex>
      {error && (
        <p role="alert">
          Не удалось изменить подключение. Перечитайте состояние и повторите
          действие.
        </p>
      )}
      {proposal && (
        <ChangeConfirmationDialog<Draft, Result>
          title={
            disconnect
              ? 'Подтверждение отключения источника'
              : proposal.draft.kind === 'pause' && proposal.draft.paused
                ? 'Подтверждение остановки загрузки'
                : 'Подтверждение возобновления сбора'
          }
          scope={connection.name}
          consequences={
            disconnect
              ? 'Чтение и выдача областей этого подключения закроются сразу, в том числе в сохранённых срезах. Учётные данные будут удалены в адаптере. Факты будут удалены по правилам класса хранения DISCONNECTED.'
              : proposal.draft.kind === 'pause' && proposal.draft.paused
                ? 'Загруженная часть сохранится как незавершённая загрузка. Полная история станет доступна после продолжения сбора и сверки.'
                : 'Сбор продолжится с сохранённой точки. Полнота истории определяется после сверки.'
          }
          dangerous={disconnect}
          initialWrite={disconnect}
          openerRef={opener}
          proposal={proposal}
          submit={async ({ draft, etag }, signal) => {
            if (draft.kind === 'disconnect') {
              await api.deleteConnection(connection.id, signal)
              return { kind: 'disconnect' }
            }
            const updated = await setConnectionPaused(
              api,
              connection.id,
              draft.paused,
              etag,
              signal,
            )
            // An older GET must not overwrite the PATCH response.
            await client.cancelQueries({ queryKey: key, exact: true })
            client.setQueryData(key, updated)
            return { kind: 'pause' }
          }}
          recheck={async ({ draft }, reason, signal) => {
            let fresh
            try {
              fresh = await readConnectionById(api, connection.id, signal)
            } catch (cause) {
              if (
                draft.kind === 'disconnect' &&
                cause instanceof ApiError &&
                cause.kind === 'not-found'
              ) {
                // DELETE is idempotent. Obtain the definitive 204 on explicit retry.
                return { kind: 'retryable' }
              }
              throw cause
            }
            await client.cancelQueries({ queryKey: key, exact: true })
            client.setQueryData(key, fresh)
            if (draft.kind === 'disconnect') return { kind: 'retryable' }
            if (reason === 'uncertain') {
              return fresh.data.sync_paused === draft.paused
                ? { kind: 'applied', result: { kind: 'pause' } }
                : { kind: 'unknown' }
            }
            if (fresh.data.status !== 'active') return { kind: 'unknown' }
            return { kind: 'not-applied', proposal: makeProposal(fresh, draft) }
          }}
          onSaved={(result) => {
            void client.invalidateQueries({ queryKey: ['connections', 'list'] })
            if (result.kind === 'disconnect') {
              void navigate(routePaths.connections, { replace: true })
            } else {
              void refresh()
            }
          }}
          onValidation={() => {
            setError(true)
            void refresh()
          }}
          onClose={() => {
            setProposal(null)
          }}
        />
      )}
    </>
  )
}
