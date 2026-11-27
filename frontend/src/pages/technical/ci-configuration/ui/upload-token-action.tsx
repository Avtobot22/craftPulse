import { Button } from '@gravity-ui/uikit'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useLayoutEffect, useRef, useState } from 'react'
import { createUploadToken } from '@/shared/api'
import type { ApiClient, UploadToken } from '@/shared/api'
import { ChangeConfirmationDialog, DialogFrame } from '@/shared/ui'
import type { ChangeAttempt, ChangeRecheck } from '@/shared/ui'
import { CopyButton } from './copy-button'

export type TokenAction =
  { kind: 'create' } | { kind: 'revoke'; token: UploadToken }

export function UploadTokenAction({
  api,
  projectId,
  action,
  opener,
  onIssued,
  onClose,
  onNotice,
}: {
  api: ApiClient
  projectId: string
  action: TokenAction
  opener: HTMLButtonElement
  onIssued: () => void
  onClose: () => void
  onNotice: (message: string) => void
}) {
  const client = useQueryClient()
  const [value, setValue] = useState<string>()
  const oneTimeValue = useRef<string>(null)
  const mounted = useRef(false)
  const openerRef = useRef<HTMLElement>(opener)
  useLayoutEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
      oneTimeValue.current = null
    }
  }, [])

  const create = useMutation({
    gcTime: 0,
    retry: false,
    mutationFn: async ({
      key,
      signal,
    }: {
      key: string
      signal: AbortSignal
    }) => {
      const issued = await createUploadToken(api, projectId, key, signal)
      // The mutation cache gets no secret, even while the dialog is open.
      if (mounted.current) oneTimeValue.current = issued.data
    },
  })

  function refreshTokens() {
    void client.invalidateQueries({
      queryKey: ['upload-tokens', 'list', projectId],
    })
  }

  function close() {
    oneTimeValue.current = null
    setValue(undefined)
    create.reset()
    onClose()
  }

  function reject(message: string) {
    onNotice(message)
    close()
  }

  async function submit(
    { draft, idempotencyKey }: ChangeAttempt<TokenAction>,
    signal: AbortSignal,
  ) {
    if (draft.kind === 'create') {
      await create.mutateAsync({ key: idempotencyKey, signal })
    } else {
      await api.revokeUploadToken(draft.token.id, signal)
    }
  }

  function recheck(
    attempt: ChangeAttempt<TokenAction>,
  ): Promise<ChangeRecheck<TokenAction, void>> {
    refreshTokens()
    // DELETE is repeatable. A lost issuance response cannot recover the one-time value.
    if (attempt.draft.kind === 'revoke')
      return Promise.resolve({ kind: 'retryable' })
    reject(
      'Не удалось подтвердить выпуск. Проверьте список токенов. Если новый токен появился, отзовите его перед повторным выпуском.',
    )
    return Promise.resolve({ kind: 'unknown' })
  }

  if (value) {
    return (
      <DialogFrame
        title="Токен загрузки выпущен"
        description="Скопируйте токен сейчас. После закрытия окна его значение нельзя посмотреть повторно."
        onCancel={close}
        openerRef={openerRef}
        actions={
          <Button view="action" onClick={close}>
            Закрыть
          </Button>
        }
      >
        <code className="cp-ci-setup__token-value">{value}</code>
        <CopyButton value={value} label="Скопировать токен" />
      </DialogFrame>
    )
  }

  const issuing = action.kind === 'create'
  return (
    <ChangeConfirmationDialog
      title={issuing ? 'Выпустить токен загрузки' : 'Отозвать токен загрузки'}
      scope={`Проект ${projectId}`}
      consequences={
        issuing
          ? 'Значение будет показано один раз.'
          : 'Следующая загрузка отчётов с этим токеном будет отклонена.'
      }
      initialWrite
      openerRef={openerRef}
      dangerous={!issuing}
      proposal={{
        etag: '',
        changes: [
          issuing
            ? {
                label: 'Токен загрузки',
                before: 'Не выпущен',
                after: 'Действует',
              }
            : {
                label: `Токен в слоте ${String(action.token.slot)}`,
                before: 'Действует',
                after: 'Отозван',
              },
        ],
        draft: action,
      }}
      submit={submit}
      recheck={recheck}
      onSaved={() => {
        refreshTokens()
        if (issuing) {
          onIssued()
        } else {
          onNotice('Токен отозван.')
        }
      }}
      onValidation={() => {
        onNotice('Сервер отклонил операцию. Проверьте, доступен ли проект.')
      }}
      onRejected={(error) => {
        if (issuing && error.problem?.status === 409) {
          refreshTokens()
          onNotice(
            'У проекта уже два действующих токена. Сначала отзовите прежний токен.',
          )
          return true
        }
        if (error.kind === 'forbidden' || error.kind === 'not-found') {
          refreshTokens()
          onNotice(
            'Токен или право на управление токенами недоступны. Обратитесь к администратору проекта.',
          )
          return true
        }
        return false
      }}
      onClose={() => {
        // Successful issuance replaces confirmation with the one-time dialog.
        const issued = oneTimeValue.current
        if (issued === null) {
          close()
        } else {
          oneTimeValue.current = null
          setValue(issued)
        }
      }}
    />
  )
}
