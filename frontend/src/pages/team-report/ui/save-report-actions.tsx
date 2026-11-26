import { Button } from '@gravity-ui/uikit'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useId, useRef, useState } from 'react'
import type { RefObject, SubmitEvent } from 'react'
import { Link } from 'react-router'
import { ApiError } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { routePaths } from '@/shared/config'
import { DialogFrame } from '@/shared/ui'
import { createSavedView, savedViewsKey } from '../api/saved-views'
import type { SavedView, SavedViewInput } from '../api/saved-views'
import { savedViewPath } from '../model/saved-view-links'
import { SavedViewFilters } from './saved-view-filters'

interface SaveDraft {
  seriesId: string
  revisionId: string
  filters: SavedViewInput['filters'] & {
    from: string
    to: string
    step: 'week' | 'month'
  }
  projectNames: string[]
}

export function SaveReportActions({
  api,
  current,
}: {
  api: ApiClient
  current: SaveDraft | undefined
}) {
  const opener = useRef<HTMLButtonElement>(null)
  const [draft, setDraft] = useState<SaveDraft>()
  const [saved, setSaved] = useState<SavedView>()

  return (
    <div className="cp-team-report__actions">
      {current && (
        <Button
          ref={opener}
          size="xl"
          onClick={() => {
            setDraft(current)
          }}
        >
          Сохранить срез
        </Button>
      )}
      <Link to={routePaths.savedViews}>Сохранённые срезы</Link>
      {saved && (
        <span role="status">
          Срез сохранён.{' '}
          <Link to={savedViewPath(saved.id)}>Открыть «{saved.name}»</Link>
        </span>
      )}
      {draft && (
        <SaveViewDialog
          api={api}
          draft={draft}
          opener={opener}
          onClose={() => {
            setDraft(undefined)
          }}
          onSaved={(view) => {
            setSaved(view)
            setDraft(undefined)
          }}
        />
      )}
    </div>
  )
}

function SaveViewDialog({
  api,
  draft,
  opener,
  onClose,
  onSaved,
}: {
  api: ApiClient
  draft: SaveDraft
  opener: RefObject<HTMLButtonElement | null>
  onClose: () => void
  onSaved: (view: SavedView) => void
}) {
  const formId = useId()
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [pinned, setPinned] = useState(false)
  const [nameError, setNameError] = useState<string>()
  const mutation = useMutation({
    mutationFn: (input: SavedViewInput) => createSavedView(api, input),
    onSuccess: (view) => {
      void queryClient.invalidateQueries({ queryKey: savedViewsKey })
      onSaved(view)
    },
  })
  const status =
    mutation.error instanceof ApiError
      ? mutation.error.problem?.status
      : undefined
  const uncertain = mutation.isError && (status === undefined || status >= 500)

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    if (mutation.isPending || uncertain) return
    const trimmed = name.trim()
    if (!trimmed || Array.from(trimmed).length > 200) {
      setNameError(
        trimmed
          ? 'Название должно содержать не более 200 символов.'
          : 'Укажите название.',
      )
      event.currentTarget.querySelector('input')?.focus()
      return
    }
    setNameError(undefined)
    mutation.mutate({
      name: trimmed,
      series_id: draft.seriesId,
      filters: draft.filters,
      pinned_revision_id: pinned ? draft.revisionId : null,
    })
  }

  return (
    <DialogFrame
      title="Сохранить срез"
      description="Задайте название среза и выберите ревизию для открытия по ссылке."
      openerRef={opener}
      onCancel={() => {
        if (!mutation.isPending) onClose()
      }}
      actions={
        <>
          <Button size="xl" disabled={mutation.isPending} onClick={onClose}>
            Отмена
          </Button>
          <Button
            size="xl"
            view="action"
            form={formId}
            type="submit"
            loading={mutation.isPending}
            disabled={mutation.isPending || uncertain}
          >
            Сохранить
          </Button>
        </>
      }
    >
      <SavedViewFilters
        {...draft.filters}
        projects={draft.projectNames}
        taskType={draft.filters.task_type}
        size={draft.filters.size_label}
        groupBy={draft.filters.group_by}
      />
      <form
        id={formId}
        className="cp-team-report__saved-form"
        noValidate
        onSubmit={submit}
      >
        <label>
          Название среза
          <input
            type="text"
            required
            disabled={mutation.isPending}
            value={name}
            aria-invalid={Boolean(nameError)}
            aria-describedby={nameError ? formId + '-name-error' : undefined}
            onChange={(event) => {
              setName(event.currentTarget.value)
              setNameError(undefined)
            }}
          />
        </label>
        {nameError && (
          <p id={formId + '-name-error'} role="alert">
            {nameError}
          </p>
        )}
        <fieldset disabled={mutation.isPending}>
          <legend>Ревизия среза</legend>
          <label>
            <input
              type="radio"
              name="revision-mode"
              checked={!pinned}
              onChange={() => {
                setPinned(false)
              }}
            />
            Показывать актуальную ревизию
          </label>
          <label>
            <input
              type="radio"
              name="revision-mode"
              checked={pinned}
              onChange={() => {
                setPinned(true)
              }}
            />
            Закрепить текущую ревизию
          </label>
        </fieldset>
        {mutation.isError && (
          <div role="alert">
            {uncertain ? (
              <>
                Не удалось подтвердить сохранение. Проверьте список перед
                повторной попыткой.{' '}
                <Link to={routePaths.savedViews}>Проверить список</Link>
              </>
            ) : (
              'Не удалось сохранить срез. Проверьте параметры и повторите попытку.'
            )}
          </div>
        )}
      </form>
    </DialogFrame>
  )
}
