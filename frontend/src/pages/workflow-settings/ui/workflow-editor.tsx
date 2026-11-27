import { useMutation } from '@tanstack/react-query'
import { Button, Text } from '@gravity-ui/uikit'
import { useRef, useState } from 'react'
import { ApiError, fieldErrorsByPointer } from '@/shared/api'
import {
  ChangeConfirmationDialog,
  EmptyState,
  ErrorState,
  LoadingState,
  SuccessState,
  UnsavedChangesDialog,
  UnavailableRedirect,
} from '@/shared/ui'
import type { ChangeProposal } from '@/shared/ui'
import type { WorkflowApi, WorkflowData } from '../api/workflow-api'
import type { CalendarDraft, MappingDraft } from '../api/workflow-responses'
import {
  calendarChanges,
  calendarDraft,
  mappingChanges,
  mappingDraft,
  suggestedRules,
} from '../model/drafts'
import { useVersionedDraft } from '../model/use-versioned-draft'
import { validateCalendar, validateMapping } from '../model/validation'
import { CalendarForm } from './calendar-form'
import { MappingForm } from './mapping-form'
import { MappingPreview } from './mapping-preview'
import { VersionHistory } from './version-history'

type Confirmation =
  | { kind: 'mapping'; proposal: ChangeProposal<MappingDraft> }
  | { kind: 'calendar'; proposal: ChangeProposal<CalendarDraft> }

export function accessLost(error: unknown) {
  return (
    error instanceof ApiError &&
    (error.kind === 'forbidden' || error.kind === 'not-found')
  )
}

export function WorkflowEditor({
  resource,
  data,
  team,
  teams,
}: {
  resource: WorkflowApi
  data: WorkflowData
  team: { id: string; name: string }
  teams: readonly { id: string; name: string }[]
}) {
  const { catalog, mappingPage, calendarPage } = data
  const formRef = useRef<HTMLFormElement>(null)
  const opener = useRef<HTMLElement | null>(null)
  const [confirmation, setConfirmation] = useState<Confirmation>()
  const [previewRequired, setPreviewRequired] = useState(false)
  const [denied, setDenied] = useState(false)
  const currentMapping = mappingPage.data.data[0]
  const currentCalendar = calendarPage.data.data[0]
  const calendar = useVersionedDraft({
    api: resource.calendar,
    initialPage: calendarPage,
    initialDraft: calendarDraft(currentCalendar),
    fromVersion: calendarDraft,
    compare: calendarChanges,
    validate: validateCalendar,
  })
  const mapping = useVersionedDraft({
    api: resource.mapping,
    initialPage: mappingPage,
    initialDraft: currentMapping
      ? mappingDraft(currentMapping)
      : mappingDraftFromCatalog(),
    fromVersion: mappingDraft,
    compare: (current, draft) => mappingChanges(current, draft, catalog),
    validate: validateMapping,
  })
  const preview = useMutation({
    mutationFn: (draft: MappingDraft) =>
      resource.preview(draft, AbortSignal.timeout(30_000)),
    onError: (error, draft) => {
      if (accessLost(error)) setDenied(true)
      if (
        draft === mapping.draft &&
        error instanceof ApiError &&
        error.kind === 'validation' &&
        error.problem
      )
        mapping.onValidation({
          problem: error.problem,
          fields: fieldErrorsByPointer(error.problem),
        })
    },
    onSuccess: () => {
      setPreviewRequired(false)
    },
  })

  if (
    denied ||
    accessLost(mapping.older.error) ||
    accessLost(calendar.older.error)
  )
    return (
      <UnavailableRedirect
        error={
          accessLost(mapping.older.error)
            ? mapping.older.error
            : calendar.older.error
        }
      />
    )

  const stale =
    preview.data !== undefined && preview.variables !== mapping.draft
  const hasSources = catalog.instances.length > 0
  const hasHistory = catalog.instances.some(
    (instance) => instance.statuses.length > 0,
  )
  const showMapping = hasSources || Boolean(mapping.current)

  function runPreview() {
    if (preview.isPending || !mapping.check()) return
    preview.mutate(mapping.draft)
  }

  function confirmMapping(button: HTMLElement) {
    const proposal = mapping.proposal()
    if (!proposal) return
    if (!preview.data?.tasks.length || stale) {
      setPreviewRequired(true)
      return
    }
    opener.current = button
    setConfirmation({ kind: 'mapping', proposal })
  }

  function confirmCalendar(button: HTMLElement) {
    const proposal = calendar.proposal()
    if (!proposal) return
    opener.current = button
    setConfirmation({ kind: 'calendar', proposal })
  }

  async function detectAccess<T>(request: Promise<T>) {
    try {
      return await request
    } catch (error) {
      if (accessLost(error)) setDenied(true)
      throw error
    }
  }

  const close = () => {
    setConfirmation(undefined)
  }
  return (
    <form
      className="cp-workflow"
      ref={formRef}
      onSubmit={(event) => {
        event.preventDefault()
      }}
      noValidate
    >
      <section aria-labelledby="workflow-mapping-heading">
        <Text as="h2" variant="header-2" id="workflow-mapping-heading">
          Соответствие workflow
        </Text>
        <p>
          {mapping.current
            ? `Действующая версия ${String(mapping.current.version_no)}`
            : 'Соответствие ещё не подтверждено'}
        </p>
        {!hasSources && <EmptyState reason="source" />}
        {hasSources && !hasHistory && <EmptyState reason="history" />}
        {showMapping && (
          <MappingForm
            draft={mapping.draft}
            issues={mapping.issues}
            catalog={catalog}
            calendars={calendar.versions}
            proposed={!mapping.current}
            teams={teams}
            onChange={mapping.edit}
          />
        )}
        <VersionHistory
          title="История сопоставления"
          versions={mapping.versions}
          {...(mapping.nextCursor
            ? {
                loadOlder: () => {
                  if (mapping.nextCursor && !mapping.older.isPending)
                    mapping.older.mutate(mapping.nextCursor)
                },
              }
            : {})}
          loading={mapping.older.isPending}
          failed={mapping.older.isError}
        />
      </section>

      <section aria-labelledby="workflow-calendar-heading">
        <Text as="h2" variant="header-2" id="workflow-calendar-heading">
          Рабочий календарь
        </Text>
        <p>
          {calendar.current
            ? `Действующая версия ${String(calendar.current.version_no)}`
            : 'Рабочий календарь ещё не подтверждён'}
        </p>
        <CalendarForm
          draft={calendar.draft}
          issues={calendar.issues}
          onChange={calendar.edit}
        />
        <div className="cp-workflow__row">
          <Button
            type="button"
            view="outlined"
            size="xl"
            onClick={(event) => {
              confirmCalendar(event.currentTarget)
            }}
          >
            Подтвердить календарь
          </Button>
        </div>
        {calendar.noChanges ? (
          <p role="status">Изменений нет</p>
        ) : (
          calendar.saved && (
            <SuccessState
              message="Календарь подтверждён"
              version={String(calendar.saved.version_no)}
              date={calendar.saved.confirmed_at}
            />
          )
        )}
        <VersionHistory
          title="История календаря"
          versions={calendar.versions}
          {...(calendar.nextCursor
            ? {
                loadOlder: () => {
                  if (calendar.nextCursor && !calendar.older.isPending)
                    calendar.older.mutate(calendar.nextCursor)
                },
              }
            : {})}
          loading={calendar.older.isPending}
          failed={calendar.older.isError}
        />
      </section>

      {hasSources && (
        <section aria-labelledby="workflow-preview-heading">
          <Text as="h2" variant="header-2" id="workflow-preview-heading">
            Предпросмотр и сверка
          </Text>
          <div className="cp-workflow__row">
            <Button
              type="button"
              view="outlined"
              size="xl"
              aria-disabled={preview.isPending}
              onClick={runPreview}
            >
              Предпросмотр
            </Button>
            {preview.data && (
              <a
                href="#workflow-preview-result"
                onClick={(event) => {
                  event.preventDefault()
                  document.getElementById('workflow-preview-result')?.focus()
                }}
              >
                Перейти к результату
              </a>
            )}
          </div>
          {preview.isPending && <LoadingState />}
          {preview.isError &&
            !(
              preview.error instanceof ApiError &&
              preview.error.kind === 'validation'
            ) && (
              <ErrorState
                reason="Не удалось получить предпросмотр"
                {...(preview.error instanceof ApiError && preview.error.problem
                  ? { correlationId: preview.error.problem.correlation_id }
                  : {})}
                onRetry={runPreview}
                retrying={preview.isPending}
              />
            )}
          {preview.data && (
            <MappingPreview
              result={preview.data}
              stale={stale}
              catalog={catalog}
              teamId={team.id}
            />
          )}
          <div className="cp-workflow__row">
            <Button
              type="button"
              view="action"
              size="xl"
              onClick={(event) => {
                confirmMapping(event.currentTarget)
              }}
            >
              Подтвердить соответствие
            </Button>
          </div>
          {previewRequired && (
            <p role="status">
              Запустите предпросмотр текущих правил и проверьте примеры задач.
            </p>
          )}
          {mapping.noChanges ? (
            <p role="status">Изменений нет</p>
          ) : (
            mapping.saved && (
              <SuccessState
                message="Соответствие подтверждено"
                version={String(mapping.saved.version_no)}
                date={mapping.saved.confirmed_at}
              />
            )
          )}
        </section>
      )}

      {confirmation?.kind === 'mapping' && (
        <ChangeConfirmationDialog
          title="Подтвердить соответствие"
          scope={team.name}
          consequences="Новая версия запустит пересчёт. Сохранённые ревизии отчёта останутся на прежней версии."
          proposal={confirmation.proposal}
          submit={(attempt, signal) =>
            detectAccess(resource.mapping.create(attempt, signal))
          }
          recheck={(attempt, reason, signal) =>
            detectAccess(mapping.recheck(attempt, reason, signal))
          }
          onSaved={mapping.accept}
          onValidation={mapping.onValidation}
          onClose={close}
          openerRef={opener}
          wide
        />
      )}
      {confirmation?.kind === 'calendar' && (
        <ChangeConfirmationDialog
          title="Подтвердить календарь"
          scope={team.name}
          consequences="Подтверждённую версию можно выбрать в форме сопоставления."
          proposal={confirmation.proposal}
          submit={(attempt, signal) =>
            detectAccess(resource.calendar.create(attempt, signal))
          }
          recheck={(attempt, reason, signal) =>
            detectAccess(calendar.recheck(attempt, reason, signal))
          }
          onSaved={calendar.accept}
          onValidation={calendar.onValidation}
          onClose={close}
          openerRef={opener}
        />
      )}
      <UnsavedChangesDialog
        formName="Настройки процесса"
        formRef={formRef}
        dirty={mapping.dirty || calendar.dirty}
        confirmationOpen={Boolean(confirmation)}
        onDiscard={() => {
          mapping.edit(
            mapping.current
              ? mappingDraft(mapping.current)
              : mappingDraftFromCatalog(),
          )
          calendar.edit(calendarDraft(calendar.current))
        }}
      />
    </form>
  )

  function mappingDraftFromCatalog(): MappingDraft {
    return {
      timezone: currentCalendar?.timezone ?? 'UTC',
      calendar_version_no: currentCalendar?.version_no ?? null,
      rules: suggestedRules(catalog),
    }
  }
}
