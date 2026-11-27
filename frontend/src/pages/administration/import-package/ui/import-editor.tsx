import { Button } from '@gravity-ui/uikit'
import { useEffect, useRef, useState } from 'react'
import { Navigate, useNavigate } from 'react-router'
import type { ApiClient, Team } from '@/shared/api'
import { adminLinks } from '@/shared/config'
import { UnsavedChangesDialog } from '@/shared/ui'
import { submitImport } from '../api/submit-import'
import { isSchema, isTaskSnapshot, schemaLabels } from '../model/import-draft'
import type { ImportDraft, ImportField } from '../model/import-draft'
import {
  fileIssue,
  submissionIssues,
  validateImport,
} from '../model/import-issues'
import type { ImportIssue } from '../model/import-issues'
import { ImportConfirmation } from './import-confirmation'
import { ImportFields } from './import-fields'
import { ImportIssueSummary } from './import-issue-summary'

type Stage =
  | { kind: 'editing' | 'confirming' | 'sending' }
  | { kind: 'received'; id: string }

export function ImportEditor({
  api,
  projects,
  initial,
  cancelTo,
  journalAllowed,
}: {
  api: ApiClient
  projects: readonly Team[]
  initial: ImportDraft
  cancelTo: string
  journalAllowed: boolean
}) {
  const navigate = useNavigate()
  const [draft, setDraft] = useState(initial)
  const [file, setFile] = useState<File>()
  const [stage, setStage] = useState<Stage>({ kind: 'editing' })
  const [issues, setIssues] = useState<ImportIssue[]>([])
  const [issueAttempt, setIssueAttempt] = useState(0)
  const form = useRef<HTMLFormElement>(null)
  const submit = useRef<HTMLButtonElement>(null)
  const request = useRef<AbortController | null>(null)
  const project = projects.find(({ id }) => id === draft.project_id)
  const dirty =
    Boolean(file) || JSON.stringify(draft) !== JSON.stringify(initial)
  const selectionIssue = file && fileIssue(file, draft)
  const visibleIssues =
    selectionIssue && !issues.some(({ field }) => field === 'file')
      ? [...issues, selectionIssue]
      : issues
  const sending = stage.kind === 'sending'
  const confirmationOpen = stage.kind === 'confirming' || sending

  useEffect(() => () => request.current?.abort(), [])

  function clearIssues(...fields: ImportField[]) {
    setIssues((previous) =>
      previous.filter(({ field }) => field && !fields.includes(field)),
    )
  }

  function edit<K extends keyof ImportDraft>(field: K, value: ImportDraft[K]) {
    setDraft((previous) => ({
      ...previous,
      ...(field === 'schema_id' ? { report_format: '' } : {}),
      [field]: value,
    }))
    if (field === 'schema_id')
      clearIssues(field, 'report_format', 'commit_sha', 'file', 'snapshot')
    else if (field === 'report_format') clearIssues(field, 'file')
    else clearIssues(field)
  }

  function showIssues(next: ImportIssue[]) {
    setIssues(next)
    setIssueAttempt((value) => value + 1)
  }

  async function confirm() {
    if (request.current || stage.kind !== 'confirming' || !file) return
    if (!project) {
      setStage({ kind: 'editing' })
      showIssues(validateImport(draft, file, false))
      return
    }
    const controller = new AbortController()
    request.current = controller
    setStage({ kind: 'sending' })
    try {
      const id = await submitImport(api, draft, file, controller.signal)
      if (!controller.signal.aborted) setStage({ kind: 'received', id })
    } catch (error) {
      if (!controller.signal.aborted) {
        setStage({ kind: 'editing' })
        showIssues(submissionIssues(error, draft))
      }
    } finally {
      request.current = null
    }
  }

  if (stage.kind === 'received') {
    return <Navigate to={adminLinks.receipt(stage.id)} replace />
  }

  return (
    <>
      <form
        ref={form}
        noValidate
        aria-labelledby="import-heading"
        className="cp-import__form"
        onSubmit={(event) => {
          event.preventDefault()
          if (stage.kind !== 'editing') return
          const errors = validateImport(draft, file, Boolean(project))
          if (errors.length) showIssues(errors)
          else {
            setIssues([])
            setStage({ kind: 'confirming' })
          }
        }}
      >
        {issues.length > 0 && (
          <ImportIssueSummary
            key={issueAttempt}
            issues={issues}
            journalAllowed={journalAllowed}
          />
        )}
        <fieldset disabled={confirmationOpen} className="cp-import__fields">
          <ImportFields
            draft={draft}
            file={file}
            projects={projects}
            issues={visibleIssues}
            onEdit={edit}
            onFile={(next) => {
              setFile(next)
              clearIssues('file', 'snapshot')
            }}
          />
        </fieldset>
        <footer className="cp-import__actions">
          <Button
            type="submit"
            size="xl"
            view="action"
            ref={submit}
            disabled={confirmationOpen}
          >
            Передать файл
          </Button>
          <Button
            size="xl"
            view="outlined"
            disabled={confirmationOpen}
            onClick={() => {
              void navigate(cancelTo)
            }}
          >
            Отмена
          </Button>
        </footer>
      </form>
      <UnsavedChangesDialog
        formName="Импорт"
        formRef={form}
        dirty={dirty}
        confirmationOpen={confirmationOpen}
        onDiscard={() => {
          setDraft(initial)
          setFile(undefined)
          setIssues([])
        }}
      />
      {confirmationOpen && file && (
        <ImportConfirmation
          project={project?.name ?? 'Проект недоступен'}
          schema={
            (isSchema(draft.schema_id)
              ? schemaLabels[draft.schema_id]
              : draft.schema_id) +
            ' · ' +
            draft.schema_version.trim()
          }
          file={file}
          snapshot={draft.snapshot || isTaskSnapshot(file, draft)}
          sending={sending}
          openerRef={submit}
          onConfirm={() => {
            void confirm()
          }}
          onClose={() => {
            setStage({ kind: 'editing' })
          }}
        />
      )}
    </>
  )
}
