import { Text } from '@gravity-ui/uikit'
import { useInfiniteQuery, useQueryClient } from '@tanstack/react-query'
import { useLayoutEffect, useRef, useState } from 'react'
import type { SubmitEvent } from 'react'
import { Link, Navigate } from 'react-router'
import type { ApiClient } from '@/shared/api'
import { routePaths } from '@/shared/config'
import {
  ChangeConfirmationDialog,
  LoadingState,
  SuccessState,
  UnsavedChangesDialog,
} from '@/shared/ui'
import type {
  ChangeAttempt,
  ChangeProposal,
  ChangeRecheck,
  RecheckReason,
} from '@/shared/ui'
import {
  createTargetVersion,
  readVersionPage,
  versionQuery,
} from '../api/target-api'
import type { TargetDraft, TargetGroup, TargetVersion } from '../api/target-api'
import {
  durationIssue,
  formatDuration,
  isAccessError,
  parseDuration,
  targetProposal,
  validationIssues,
} from '../model/target-settings'
import type {
  FormIssue,
  HistoricalBasis,
  TargetInput,
} from '../model/target-settings'
import { TargetForm } from './target-form'
import { TargetHistory } from './target-history'
import { TargetReadError } from './target-read-error'

interface EditDraft extends TargetInput {
  base: { etag: string; current: TargetVersion | undefined }
}

interface TargetEditorProps {
  api: ApiClient
  teamId: string
  group: TargetGroup
  historical: HistoricalBasis | null
  reportLink: string
  refreshBaseline: () => void
}

export function TargetEditor({
  api,
  teamId,
  group,
  historical,
  reportLink,
  refreshBaseline,
}: TargetEditorProps) {
  const options = versionQuery(api, teamId, group)
  const history = useInfiniteQuery(options)
  const client = useQueryClient()
  const queryKey = options.queryKey
  const [draft, setDraft] = useState<EditDraft | null>(null)
  const [issues, setIssues] = useState<FormIssue[]>([])
  const [proposal, setProposal] = useState<ChangeProposal<TargetDraft> | null>(
    null,
  )
  const [saved, setSaved] = useState<TargetVersion | null>(null)
  const formRef = useRef<HTMLFormElement>(null)
  const headingRef = useRef<HTMLHeadingElement>(null)

  useLayoutEffect(() => {
    headingRef.current?.focus()
  }, [])

  const firstPage = history.data?.pages[0]
  const current = firstPage?.data.data[0]
  const versions = history.data?.pages.flatMap((page) => page.data.data) ?? []

  function edit(change: Partial<TargetInput>) {
    if (!firstPage) return
    const next = {
      ...(draft ?? {
        days: '',
        hours: '',
        historical: null,
        base: { etag: firstPage.etag, current },
      }),
      ...change,
    }
    setDraft(next.days || next.hours || next.historical ? next : null)
    setIssues([])
    setSaved(null)
  }

  function preview(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!firstPage || (history.isError && !history.isFetchNextPageError)) return
    const seconds = parseDuration(draft?.days ?? '', draft?.hours ?? '')
    if (seconds === null || !draft) {
      setIssues([durationIssue])
      return
    }
    const basis = draft.historical
    setIssues([])
    setProposal(
      targetProposal(
        {
          ...group,
          duration_seconds: seconds,
          basis: basis ? 'historical_p85' : 'manual',
          basis_revision_id: basis?.revision ?? null,
          basis_period: basis?.period ?? null,
        },
        draft.base.etag,
        draft.base.current,
      ),
    )
  }

  async function recheck(
    attempt: ChangeAttempt<TargetDraft>,
    reason: RecheckReason,
    signal: AbortSignal,
  ): Promise<ChangeRecheck<TargetDraft, TargetVersion>> {
    await client.cancelQueries({ queryKey })
    const latest = await readVersionPage(api, teamId, group, signal)
    signal.throwIfAborted()
    client.setQueryData(queryKey, { pages: [latest], pageParams: [undefined] })
    if (latest.etag === attempt.etag) {
      return reason === 'uncertain'
        ? { kind: 'retryable' }
        : { kind: 'unknown' }
    }
    // A newer version alone cannot identify the outcome of a timed-out write.
    if (reason === 'uncertain') return { kind: 'unknown' }

    const base = { etag: latest.etag, current: latest.data.data[0] }
    setDraft((input) => input && { ...input, base })
    return {
      kind: 'not-applied',
      proposal: targetProposal(attempt.draft, base.etag, base.current),
    }
  }

  function onSaved(version: TargetVersion) {
    setDraft(null)
    setIssues([])
    setSaved(version)
    void client.cancelQueries({ queryKey }).then(() => {
      client.setQueryData(queryKey, (data: typeof history.data) => {
        const page = data?.pages[0]
        if (!page || Number(page.etag.slice(1, -1)) >= version.version_no)
          return data
        return {
          ...data,
          pages: [
            {
              etag: `"${String(version.version_no)}"`,
              data: { ...page.data, data: [version, ...page.data.data] },
            },
            ...data.pages.slice(1),
          ],
        }
      })
      // A different author may have confirmed another version after the write.
      void client.invalidateQueries({ queryKey })
    })
  }

  if (isAccessError(history.error))
    return <Navigate to={routePaths.unavailable} replace />

  return (
    <section
      className="cp-targets__panel"
      aria-labelledby="target-group-heading"
    >
      <h2 ref={headingRef} tabIndex={-1} id="target-group-heading">
        <Text variant="subheader-3">
          Срок группы {group.task_type}, {group.size_label}
        </Text>
      </h2>
      {history.isPending && <LoadingState />}
      {history.isError && (
        <TargetReadError
          error={history.error}
          reason="Не удалось загрузить версии срока группы."
          retry={() => {
            void (history.isFetchNextPageError
              ? history.fetchNextPage()
              : history.refetch())
          }}
          retrying={history.isFetching}
          updatedAt={history.dataUpdatedAt}
        />
      )}
      {firstPage && (
        <>
          {current && (
            <p>
              Действующий срок - {formatDuration(current.duration_seconds)},
              версия v{current.version_no}.
            </p>
          )}
          {saved && (
            <>
              <SuccessState
                message="Целевой срок подтверждён"
                version={`v${String(saved.version_no)}`}
                date={saved.confirmed_at}
              />
              <Link to={reportLink}>Перейти к отчёту команды</Link>
            </>
          )}
          <TargetForm
            formRef={formRef}
            issues={issues}
            input={draft}
            historical={historical}
            disabled={history.isError && !history.isFetchNextPageError}
            onSubmit={preview}
            edit={edit}
            refreshBaseline={refreshBaseline}
          />
          <TargetHistory
            versions={versions}
            {...(history.hasNextPage
              ? {
                  loadMore: () => {
                    void history.fetchNextPage()
                  },
                }
              : {})}
            loading={history.isFetching}
          />
        </>
      )}
      {firstPage && (
        <UnsavedChangesDialog
          formName="Целевой срок"
          formRef={formRef}
          dirty={draft !== null}
          confirmationOpen={proposal !== null}
          onDiscard={() => {
            setDraft(null)
            setIssues([])
          }}
        />
      )}
      {proposal && (
        <ChangeConfirmationDialog
          title="Подтверждение целевого срока"
          scope={`${group.task_type}, ${group.size_label}`}
          consequences="Новая версия применяется к будущим расчётам. Прежние нарушения сохраняют свою версию срока."
          proposal={proposal}
          submit={(attempt, signal) =>
            createTargetVersion(api, teamId, attempt, signal)
          }
          recheck={recheck}
          onSaved={onSaved}
          onValidation={(validation) => {
            setIssues(validationIssues(validation))
          }}
          onClose={() => {
            setProposal(null)
          }}
        />
      )}
    </section>
  )
}
