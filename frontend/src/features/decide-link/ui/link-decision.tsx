import { Button, Flex } from '@gravity-ui/uikit'
import { useLayoutEffect, useRef, useState } from 'react'
import type { ApiClient } from '@/shared/api'
import { DialogFrame } from '@/shared/ui'
import { decisionFailure } from '../model/link-decision'
import type { LinkDecisionSelection } from '../model/link-decision'

const requestTimeoutMs = 30_000
const actionLabels = { confirm: 'Подтвердить связь', reject: 'Отклонить связь' }

export function LinkDecisionActions({
  linkId,
  targetLabel,
  onSelect,
}: {
  linkId: string
  targetLabel: string
  onSelect: (selection: LinkDecisionSelection) => void
}) {
  return (
    <Flex gap="spacing-3" wrap="wrap">
      {(['confirm', 'reject'] as const).map((decision) => (
        <Button
          key={decision}
          size="l"
          view={decision === 'reject' ? 'outlined-danger' : 'outlined'}
          onClick={(event) => {
            onSelect({
              linkId,
              targetLabel,
              body: { decision },
              idempotencyKey: crypto.randomUUID(),
              openerRef: { current: event.currentTarget },
            })
          }}
        >
          {actionLabels[decision]}
        </Button>
      ))}
    </Flex>
  )
}

type RefreshReason = 'saved' | 'conflict' | 'uncertain'
type Status =
  | {
      kind:
        | 'preview'
        | 'sending'
        | 'retryable'
        | 'conflict'
        | 'forbidden'
        | 'rejected'
    }
  | { kind: 'refreshing' | 'refresh-failed'; reason: RefreshReason }

const decisionErrors: Partial<Record<Status['kind'], string>> = {
  forbidden: 'Действие недоступно. Нет права принимать решения по связям.',
  rejected: 'Не удалось сохранить решение.',
  retryable:
    'Ответ на запрос не получен. Можно повторить запрос с тем же ключом.',
  'refresh-failed': 'Не удалось перечитать карточку задачи.',
}

export function LinkDecisionDialog({
  api,
  selection,
  taskLabel,
  refreshTask,
  onForbidden,
  onNotFound,
  onClose,
}: {
  api: ApiClient
  selection: LinkDecisionSelection
  taskLabel: string
  refreshTask: () => Promise<void>
  onForbidden: () => void
  onNotFound: () => void
  onClose: () => void
}) {
  const [status, setStatus] = useState<Status>({ kind: 'preview' })
  const active = useRef(false)
  const locked = useRef(false)
  const errorRef = useRef<HTMLDivElement>(null)
  const working = status.kind === 'sending' || status.kind === 'refreshing'
  const title = actionLabels[selection.body.decision]

  useLayoutEffect(() => {
    active.current = true
    return () => {
      active.current = false
    }
  }, [])
  useLayoutEffect(() => {
    if (!working) {
      locked.current = false
      errorRef.current?.focus()
    }
  }, [working, status])

  async function refresh(reason: RefreshReason) {
    setStatus({ kind: 'refreshing', reason })
    try {
      await refreshTask()
    } catch {
      if (active.current) setStatus({ kind: 'refresh-failed', reason })
      return
    }
    if (!active.current) return
    if (reason === 'saved') onClose()
    else setStatus({ kind: reason === 'uncertain' ? 'retryable' : 'conflict' })
  }

  async function submit() {
    setStatus({ kind: 'sending' })
    try {
      await api.decideLink(
        selection.linkId,
        selection.body,
        selection.idempotencyKey,
        AbortSignal.timeout(requestTimeoutMs),
      )
    } catch (error) {
      if (!active.current) return
      const failure = decisionFailure(error)
      switch (failure) {
        case 'conflict':
        case 'uncertain':
          await refresh(failure)
          break
        case 'forbidden':
          onForbidden()
          setStatus({ kind: 'forbidden' })
          break
        case 'not-found':
          onNotFound()
          break
        case 'session-ended':
          onClose()
          break
        case 'rejected':
          setStatus({ kind: 'rejected' })
          break
      }
      return
    }
    if (active.current) await refresh('saved')
  }

  function run(action: () => Promise<void>) {
    if (locked.current) return
    locked.current = true
    void action()
  }

  function dismiss() {
    if (!locked.current) onClose()
  }

  const conflict =
    status.kind === 'conflict' ||
    ('reason' in status && status.reason === 'conflict')
  const terminal =
    status.kind === 'conflict' ||
    status.kind === 'forbidden' ||
    status.kind === 'rejected'
  const errorMessage = decisionErrors[status.kind]
  return (
    <DialogFrame
      title={title}
      description="Проверьте задачу и запрос на слияние перед сохранением."
      openerRef={selection.openerRef}
      onCancel={dismiss}
      actions={
        <>
          <Button
            size="xl"
            view="outlined"
            disabled={working}
            onClick={dismiss}
          >
            {terminal ? 'Закрыть' : 'Отмена'}
          </Button>
          {(status.kind === 'preview' ||
            status.kind === 'retryable' ||
            status.kind === 'sending') && (
            <Button
              size="xl"
              view="action"
              disabled={working}
              onClick={() => {
                run(submit)
              }}
            >
              {status.kind === 'retryable' ? 'Повторить запрос' : title}
            </Button>
          )}
          {status.kind === 'refresh-failed' && (
            <Button
              size="xl"
              view="action"
              onClick={() => {
                run(() => refresh(status.reason))
              }}
            >
              Перечитать карточку
            </Button>
          )}
        </>
      }
    >
      {working && (
        <p role="status">
          {status.kind === 'sending'
            ? 'Сохраняем решение'
            : 'Перечитываем карточку'}
        </p>
      )}
      {(conflict || errorMessage) && (
        <div role="alert" ref={errorRef} tabIndex={-1}>
          {conflict && <p>Связь уже подтверждена источником.</p>}
          {errorMessage && <p>{errorMessage}</p>}
        </div>
      )}
      <dl className="cp-settings-dialog__details">
        <dt>Задача</dt>
        <dd>{taskLabel}</dd>
        <dt>Запрос на слияние</dt>
        <dd>{selection.targetLabel}</dd>
        <dt>Последствия</dt>
        <dd>
          {selection.body.decision === 'confirm'
            ? 'MR станет основанием наблюдений задачи.'
            : 'Кандидат уйдёт из оснований наблюдений задачи.'}
        </dd>
      </dl>
    </DialogFrame>
  )
}
