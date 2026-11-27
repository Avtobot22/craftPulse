import { Alert, Button } from '@gravity-ui/uikit'
import { useQueryClient } from '@tanstack/react-query'
import { useLayoutEffect, useRef, useState } from 'react'
import {
  createCheckProfileVersion,
  getCheckProfile,
  isStrongETag,
  readCurrentCheckProfile,
} from '@/shared/api'
import type { ApiClient, ApiResponse, CheckProfileVersion } from '@/shared/api'
import { ChangeConfirmationDialog, UnsavedChangesDialog } from '@/shared/ui'
import type { ChangeAttempt, ChangeProposal, ChangeRecheck } from '@/shared/ui'
import {
  compareDraft,
  draftFrom,
  rebaseDraft,
  rowLabel,
  validationMessages,
} from '../model/matrix'
import type { ProfileDraft } from '../model/matrix'
import { MatrixTable } from './matrix-table'

type Resource = ApiResponse<CheckProfileVersion>
interface Edit {
  base: Resource
  draft: ProfileDraft
}

export function MatrixEditor({
  api,
  projectId,
  current,
}: {
  api: ApiClient
  projectId: string
  current: Resource
}) {
  const client = useQueryClient()
  const queryKey = getCheckProfile(api, projectId).queryKey
  const formRef = useRef<HTMLFormElement>(null)
  const errorRef = useRef<HTMLDivElement>(null)
  const [edit, setEdit] = useState<Edit>()
  const [proposal, setProposal] = useState<ChangeProposal<ProfileDraft>>()
  const [messages, setMessages] = useState<string[]>([])
  const [removed, setRemoved] = useState<string[]>([])
  const submittedDraft = useRef<ProfileDraft>(null)
  useLayoutEffect(() => {
    if (messages.length) errorRef.current?.focus()
  }, [messages])

  const base = edit?.base ?? current
  const draft = edit?.draft ?? draftFrom(base.data)
  const changes = draft ? compareDraft(base.data, draft) : []
  const dirty = changes.length > 0

  function adopt(resource: Resource) {
    client.setQueryData(queryKey, resource)
    setEdit(undefined)
    setMessages([])
    setRemoved([])
    void client.invalidateQueries({
      queryKey: ['check-profiles', 'ci-configuration', projectId],
    })
  }

  async function recheck(
    attempt: ChangeAttempt<ProfileDraft>,
    reason: 'conflict' | 'uncertain',
    signal: AbortSignal,
  ): Promise<ChangeRecheck<ProfileDraft, Resource>> {
    const latest = await readCurrentCheckProfile(api, projectId, signal)
    if (!isStrongETag(latest.etag)) return { kind: 'unknown' }
    if (latest.etag === attempt.etag) {
      return reason === 'uncertain'
        ? { kind: 'retryable' }
        : { kind: 'unknown' }
    }
    const currentDraft = draftFrom(latest.data)
    if (!currentDraft) {
      adopt(latest)
      setMessages([
        'Актуальная версия содержит неподдерживаемые строки. Сохранение недоступно.',
      ])
      setProposal(undefined)
      return { kind: 'unknown' }
    }

    // Reload the confirmed matrix, then apply only the choices in the previous preview.
    const rebased = rebaseDraft(base.data, attempt.draft, currentDraft)
    const changes = compareDraft(latest.data, rebased.draft)
    if (
      reason === 'uncertain' &&
      changes.length === 0 &&
      rebased.removed.length === 0
    ) {
      return { kind: 'applied', result: latest }
    }
    if (reason === 'uncertain') return { kind: 'retryable' }

    client.setQueryData(queryKey, latest)
    setEdit({ base: latest, draft: rebased.draft })
    setRemoved(rebased.removed.map(rowLabel))
    return {
      kind: 'not-applied',
      proposal: { draft: rebased.draft, changes, etag: latest.etag },
    }
  }

  return (
    <>
      <p>Подтверждённая версия {base.data.version_no}</p>
      <form
        ref={formRef}
        onSubmit={(event) => {
          event.preventDefault()
          if (draft && dirty && isStrongETag(base.etag)) {
            setMessages([])
            setProposal({ etag: base.etag, changes, draft })
          }
        }}
      >
        {!draft && (
          <p role="status">
            Матрица содержит неподдерживаемые строки. Сохранение недоступно.
          </p>
        )}
        {messages.length > 0 && (
          <div role="alert" tabIndex={-1} ref={errorRef}>
            <Alert
              theme="danger"
              title="Матрица не прошла проверку"
              message={
                <ul>
                  {messages.map((message) => (
                    <li key={message}>{message}</li>
                  ))}
                </ul>
              }
            />
          </div>
        )}
        {removed.length > 0 && (
          <p role="status">
            В актуальной версии удалены строки: {removed.join('; ')}
          </p>
        )}
        <MatrixTable
          version={base.data}
          draft={draft}
          onChange={(index, selection) => {
            if (!draft) return
            setMessages([])
            setEdit({
              base,
              draft: {
                entries: draft.entries.map((row, i) =>
                  i === index ? { ...row, ...selection } : row,
                ),
              },
            })
          }}
        />
        {!isStrongETag(base.etag) && (
          <p>Версия для сохранения недоступна. Обновите страницу.</p>
        )}
        <Button
          type="submit"
          disabled={!dirty || !isStrongETag(base.etag)}
          view="action"
        >
          Подтвердить профиль
        </Button>
      </form>
      <UnsavedChangesDialog
        formName="Матрица проверок"
        formRef={formRef}
        dirty={dirty}
        confirmationOpen={Boolean(proposal)}
        onDiscard={() => {
          setEdit(undefined)
        }}
      />
      {proposal && (
        <ChangeConfirmationDialog
          title="Подтвердить профиль проверок"
          scope={`Проект ${projectId}`}
          consequences="Новая версия применяется к будущим проверкам. Конфигурацию CI обновите отдельным MR."
          proposal={proposal}
          submit={(attempt, signal) => {
            submittedDraft.current = attempt.draft
            return createCheckProfileVersion(api, projectId, attempt.draft, {
              ifMatch: attempt.etag,
              idempotencyKey: attempt.idempotencyKey,
              signal,
            })
          }}
          recheck={recheck}
          onRejected={(error) => {
            if (error.kind !== 'forbidden') return false
            setMessages(['У вас больше нет права изменять матрицу проверок.'])
            return true
          }}
          onSaved={adopt}
          onValidation={(validation) => {
            const messages = submittedDraft.current
              ? validationMessages(validation, submittedDraft.current)
              : []
            setMessages(
              messages.length
                ? messages
                : ['Проверьте выбранные режимы и профили.'],
            )
          }}
          onClose={() => {
            setProposal(undefined)
            submittedDraft.current = null
          }}
        />
      )}
    </>
  )
}
