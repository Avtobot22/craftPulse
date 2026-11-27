import { Alert, Button } from '@gravity-ui/uikit'
import { useQueryClient } from '@tanstack/react-query'
import { useLayoutEffect, useRef, useState } from 'react'
import { generatePath, Link, useSearchParams } from 'react-router'
import {
  exceptionOrigin,
  exceptionStatus,
  invalidateExceptionViews,
} from '@/entities/exception'
import {
  createException,
  createExceptionVersion,
  isStrongETag,
  readExceptionById,
} from '@/shared/api'
import type {
  ApiClient,
  ApiResponse,
  ExceptionDetail,
  ExceptionDraft,
} from '@/shared/api'
import { routePaths } from '@/shared/config'
import { ChangeConfirmationDialog, UnsavedChangesDialog } from '@/shared/ui'
import type { ChangeProposal } from '@/shared/ui'
import {
  fieldNames,
  initialFields,
  previewDraft,
  responseIssues,
  validateDraft,
} from '../model/exception-draft'
import type {
  ExceptionFields as Fields,
  FormIssues,
  ScopeOption,
} from '../model/exception-draft'
import { ExceptionFields } from './exception-fields'

const forbidden =
  'Исключение может оформить техлид или разработчик с правом на эту область.'

export function ExceptionEditor({
  api,
  scopes: initialScopes,
  existing,
  findingId,
}: {
  api: ApiClient
  scopes: ScopeOption[]
  existing: ApiResponse<ExceptionDetail> | undefined
  findingId: string | null
}) {
  const [search] = useSearchParams()
  const queries = useQueryClient()
  const [scopes] = useState(initialScopes)
  const [baseline, setBaseline] = useState(existing)
  const [initial] = useState(() =>
    initialFields(existing?.data.current_version),
  )
  const [fields, setFields] = useState(initial)
  const [issues, setIssues] = useState<FormIssues>({})
  const [proposal, setProposal] = useState<ChangeProposal<ExceptionDraft>>()
  const [result, setResult] = useState<ExceptionDetail>()
  const form = useRef<HTMLFormElement>(null)
  // Keep the original key if the user closes an uncertain confirmation
  // and submits the same draft again.
  const createKeys = useRef(new Map<string, string>())
  const opener = useRef<HTMLButtonElement>(null)
  const errorSummary = useRef<HTMLDivElement>(null)
  const resultHeading = useRef<HTMLHeadingElement>(null)

  useLayoutEffect(() => {
    errorSummary.current?.focus()
  }, [issues])
  useLayoutEffect(() => {
    resultHeading.current?.focus()
  }, [result])

  const exceptionId = baseline?.data.id
  const dirty = Object.keys(initial).some(
    (key) => fields[key as keyof Fields] !== initial[key as keyof Fields],
  )

  function openConfirmation() {
    const valid = validateDraft(fields, scopes[fields.scopeIndex]?.value)
    if ('issues' in valid) {
      setIssues(valid.issues)
      return
    }
    setIssues({})
    setProposal({
      etag: baseline?.etag ?? '',
      draft: valid.draft,
      changes: previewDraft(valid.draft, baseline?.data.current_version),
    })
  }

  const origin = exceptionOrigin(search, findingId)
  const history = (id: string) => {
    const params = new URLSearchParams(search)
    if (findingId) params.set('origin_finding', findingId)
    return {
      pathname: generatePath(routePaths.exceptions, { exceptionId: id }),
      search: params.toString(),
    }
  }

  if (result) {
    const pending =
      result.current_version.status === 'pending' && result.state !== 'expired'
    return (
      <section>
        <h2 ref={resultHeading} tabIndex={-1}>
          {exceptionStatus(pending ? 'pending' : result.state)}
        </h2>
        {pending && (
          <p>
            Эта версия начнёт действовать после подтверждения и наступления
            срока действия.
          </p>
        )}
        <div className="cp-exception-form__actions">
          <Link to={history(result.id)}>Открыть историю решения</Link>
          {origin && <Link to={origin}>К исходной находке</Link>}
        </div>
      </section>
    )
  }

  return (
    <>
      <form
        ref={form}
        noValidate
        onSubmit={(event) => {
          event.preventDefault()
          openConfirmation()
        }}
      >
        {Object.keys(issues).length > 0 && (
          <div
            id="exception-issues"
            ref={errorSummary}
            tabIndex={-1}
            role="alert"
          >
            {issues.general && <p>{issues.general}</p>}
            <ul>
              {Object.entries(fieldNames).map(([name, label]) => {
                const message = issues[name as keyof typeof fieldNames]
                return (
                  message && (
                    <li key={name}>
                      <a href={'#' + name}>{label}</a>
                      {' · '}
                      {message}
                    </li>
                  )
                )
              })}
            </ul>
          </div>
        )}
        <ExceptionFields
          fields={fields}
          scopes={scopes}
          issues={issues}
          edit={(patch) => {
            setFields({ ...fields, ...patch })
            setIssues({})
          }}
        />
        <div className="cp-exception-form__actions">
          <Button ref={opener} type="submit" view="action" size="l">
            Сохранить
          </Button>
          {origin && <Link to={origin}>Отмена</Link>}
          {!origin && exceptionId && (
            <Link to={history(exceptionId)}>Отмена</Link>
          )}
        </div>
      </form>
      {proposal && (
        <ChangeConfirmationDialog
          title={
            exceptionId ? 'Сохранить новую версию' : 'Сохранить исключение'
          }
          scope={scopes[fields.scopeIndex]?.label ?? 'Исключение'}
          consequences={
            exceptionId
              ? 'Прежняя версия останется в истории. Если нужно ревью, новая версия дождётся подтверждения.'
              : 'Если политика требует ревью, решение дождётся подтверждения.'
          }
          proposal={proposal}
          initialWrite={!exceptionId}
          openerRef={opener}
          submit={(attempt, signal) => {
            if (exceptionId)
              return createExceptionVersion(
                api,
                exceptionId,
                attempt.draft,
                attempt.etag,
                attempt.idempotencyKey,
                signal,
              )
            const payload = JSON.stringify(attempt.draft)
            const key =
              createKeys.current.get(payload) ?? attempt.idempotencyKey
            createKeys.current.set(payload, key)
            return createException(api, attempt.draft, key, signal)
          }}
          recheck={async (attempt, reason, signal) => {
            if (!exceptionId)
              return reason === 'uncertain'
                ? { kind: 'retryable' }
                : { kind: 'unknown' }
            const latest = await readExceptionById(api, exceptionId, signal)
            signal.throwIfAborted()
            if (reason === 'uncertain') return { kind: 'retryable' }
            if (!isStrongETag(latest.etag) || latest.etag === attempt.etag)
              return { kind: 'unknown' }
            setBaseline(latest)
            return {
              kind: 'not-applied',
              proposal: {
                etag: latest.etag,
                changes: previewDraft(
                  attempt.draft,
                  latest.data.current_version,
                ),
                draft: attempt.draft,
              },
            }
          }}
          onSaved={({ data }) => {
            invalidateExceptionViews(queries)
            setResult(data)
          }}
          onValidation={({ fields: errors }) => {
            setIssues(responseIssues(errors))
          }}
          onRejected={(error) => {
            if (error.problem?.status !== 403) return false
            setIssues({ general: forbidden })
            return true
          }}
          onClose={() => {
            setProposal(undefined)
          }}
          wide
        />
      )}
      {baseline && baseline !== existing && (
        <Alert
          theme="warning"
          title="Загружена актуальная версия"
          message={
            <Link to={history(baseline.data.id)}>
              Посмотреть актуальное решение
            </Link>
          }
        />
      )}
      <UnsavedChangesDialog
        formName="Исключение"
        formRef={form}
        dirty={dirty || Boolean(proposal)}
        confirmationOpen={Boolean(proposal)}
        onDiscard={() => {
          setFields(initial)
        }}
      />
    </>
  )
}
