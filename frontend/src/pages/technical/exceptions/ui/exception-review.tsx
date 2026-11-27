import { Alert, Button } from '@gravity-ui/uikit'
import { useQueryClient } from '@tanstack/react-query'
import { useLayoutEffect, useRef, useState } from 'react'
import {
  describeExceptionScope,
  invalidateExceptionViews,
} from '@/entities/exception'
import {
  ApiError,
  decideException,
  getException,
  readExceptionById,
} from '@/shared/api'
import type {
  ApiClient,
  ExceptionDecision,
  ExceptionDetail,
} from '@/shared/api'
import { contractLabel, technicalLabels } from '@/shared/config'
import { ChangeConfirmationDialog } from '@/shared/ui'
import type { ChangeProposal } from '@/shared/ui'

type ReviewOutcome =
  { kind: 'saved'; data: ExceptionDetail } | { kind: 'stale' | 'conflict' }

export function ExceptionReview({
  api,
  exception,
  etag,
  refreshing,
  approve,
  reject,
  withdraw,
  onResult,
}: {
  api: ApiClient
  exception: ExceptionDetail
  etag: string | null
  refreshing: boolean
  approve: boolean
  reject: boolean
  withdraw: boolean
  onResult: () => void
}) {
  const queries = useQueryClient()
  const [proposal, setProposal] = useState<
    ChangeProposal<ExceptionDecision> & { scope: string }
  >()
  const [notice, setNotice] = useState('')
  const [choice, setChoice] = useState<'approve' | 'reject' | ''>('')
  const [comment, setComment] = useState('')
  const opener = useRef<HTMLElement>(null)
  const focusResult = useRef(false)
  const version = exception.current_version

  useLayoutEffect(() => {
    if (!proposal && focusResult.current) {
      focusResult.current = false
      onResult()
    }
  }, [proposal, onResult])

  function confirm(decision: ExceptionDecision['decision']) {
    if (!etag || refreshing) return
    setNotice('')
    const text = decision === 'withdraw' ? '' : comment.trim()
    setProposal({
      etag,
      scope: describeExceptionScope(version.scope),
      draft: { decision, ...(text ? { comment: text } : {}) },
      changes: [
        {
          label: `Версия ${String(version.version_no)}`,
          before: contractLabel(
            technicalLabels.exceptionVersionStatus,
            version.status,
          ),
          after: technicalLabels.reviewerDecision[decision],
        },
        ...(text
          ? [{ label: 'Комментарий', before: 'Не задан', after: text }]
          : []),
      ],
    })
  }

  const canReview = approve || reject
  const canSubmit = Boolean(etag && !refreshing)
  return (
    <>
      {notice && (
        <div role="alert">
          <Alert theme="warning" message={notice} />
        </div>
      )}
      {canReview && (
        <form
          className="cp-exceptions__review"
          onSubmit={(event) => {
            event.preventDefault()
            if (
              (choice === 'approve' && approve) ||
              (choice === 'reject' && reject)
            )
              confirm(choice)
            else setNotice('Выберите решение.')
          }}
        >
          <label>
            Решение ревьюера
            <select
              value={choice}
              onChange={(event) => {
                const value = event.target.value
                setChoice(
                  value === 'approve' || value === 'reject' ? value : '',
                )
              }}
            >
              <option value="">Выберите решение</option>
              {approve && <option value="approve">Подтвердить</option>}
              {reject && <option value="reject">Отклонить</option>}
            </select>
          </label>
          <label>
            Комментарий (необязательно)
            <textarea
              rows={2}
              value={comment}
              maxLength={2000}
              onChange={(event) => {
                setComment(event.target.value)
              }}
            />
          </label>
          <Button
            type="submit"
            view="action"
            disabled={!canSubmit}
            onFocus={(event) => {
              opener.current = event.currentTarget
            }}
          >
            Проверить решение
          </Button>
        </form>
      )}
      {withdraw && (
        <Button
          view="outlined"
          disabled={!canSubmit}
          onClick={(event) => {
            opener.current = event.currentTarget
            confirm('withdraw')
          }}
        >
          Отозвать
        </Button>
      )}
      {(canReview || withdraw) && !etag && !refreshing && (
        <p role="status">Версия недоступна. Обновите карточку решения.</p>
      )}
      {proposal && (
        <ChangeConfirmationDialog
          title={`${technicalLabels.reviewerDecision[proposal.draft.decision]} версию решения`}
          scope={proposal.scope}
          consequences="Действие появится в истории решения."
          proposal={proposal}
          openerRef={opener}
          dangerous={proposal.draft.decision !== 'approve'}
          submit={async (attempt, signal): Promise<ReviewOutcome> => {
            try {
              const { data } = await decideException(
                api,
                exception.id,
                attempt.draft,
                attempt.etag,
                attempt.idempotencyKey,
                signal,
              )
              return { kind: 'saved', data }
            } catch (error) {
              if (error instanceof ApiError && error.problem?.status === 409)
                return { kind: 'conflict' }
              if (error instanceof ApiError && error.problem?.status === 412)
                return { kind: 'stale' }
              throw error
            }
          }}
          recheck={async (_attempt, reason, signal) => {
            await readExceptionById(api, exception.id, signal)
            return reason === 'uncertain'
              ? { kind: 'retryable' }
              : { kind: 'unknown' }
          }}
          onSaved={(outcome) => {
            focusResult.current = true
            if (outcome.kind === 'saved') {
              queries.setQueryData(getException(api, exception.id).queryKey, {
                data: outcome.data,
                etag: null,
              })
              setChoice('')
              setComment('')
            } else {
              setNotice(
                outcome.kind === 'conflict'
                  ? 'Решение уже в конечном состоянии.'
                  : 'Версия изменилась. Загружаем актуальное решение.',
              )
            }
            invalidateExceptionViews(queries)
          }}
          onValidation={() => {
            setNotice('Проверьте выбор решения и комментарий.')
          }}
          onRejected={(error) => {
            if (error.problem?.status !== 403) return false
            setNotice(
              'У вас нет права выполнить это действие в области решения.',
            )
            invalidateExceptionViews(queries)
            return true
          }}
          onClose={() => {
            setProposal(undefined)
          }}
        />
      )}
    </>
  )
}
