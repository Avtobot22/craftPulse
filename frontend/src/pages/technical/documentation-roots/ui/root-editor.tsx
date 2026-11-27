import { Button } from '@gravity-ui/uikit'
import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { SubmitEvent } from 'react'
import {
  isStrongETag,
  readDocumentationRoots,
  replaceDocumentationRoots,
  listDocumentationRoots,
  serviceDocumentPages,
} from '@/shared/api'
import type {
  ApiClient,
  ApiResponse,
  DocumentationRoots,
  DocumentationRootInput,
} from '@/shared/api'
import { ChangeConfirmationDialog, UnsavedChangesDialog } from '@/shared/ui'
import type { ChangeProposal } from '@/shared/ui'
import {
  gitProfile,
  rootFields,
  rootProposal,
  validationIssues,
  validationTarget,
} from '../model/root-set'
import type { RootFields as Fields } from '../model/root-set'
import { RootFields } from './root-fields'

interface DraftRow {
  key: string
  fields: Fields
}

const newRow = (): DraftRow => ({
  key: crypto.randomUUID(),
  fields: {
    area_id: '',
    path_prefix: '',
    profile_id: '',
    owner_person_id: null,
    audience: '',
  },
})

function toRows(roots: DocumentationRoots): DraftRow[] {
  return roots.data.map((root) => ({ key: root.id, fields: rootFields(root) }))
}

export function RootEditor({
  api,
  serviceId,
  serviceName,
  initial,
  repositories,
  owners,
  onSaved,
}: {
  api: ApiClient
  serviceId: string
  serviceName: string
  initial: ApiResponse<DocumentationRoots>
  repositories: ReadonlyMap<string, string> | undefined
  owners: ReadonlyMap<string, string>
  onSaved: () => void
}) {
  const cache = useQueryClient()
  const [baseline, setBaseline] = useState(initial)
  const [rows, setRows] = useState(() => toRows(initial.data))
  const [proposal, setProposal] =
    useState<ChangeProposal<DocumentationRootInput[]>>()
  const [issues, setIssues] = useState<ReturnType<typeof validationIssues>>([])
  const [saved, setSaved] = useState(false)
  const form = useRef<HTMLFormElement>(null)
  const submitButton = useRef<HTMLButtonElement>(null)
  const errors = useRef<HTMLDivElement>(null)
  const dirty =
    JSON.stringify(rows.map(({ fields }) => fields)) !==
    JSON.stringify(baseline.data.data.map(rootFields))

  useLayoutEffect(() => {
    if (issues.length) errors.current?.focus()
  }, [issues])

  function edit(next: DraftRow[]) {
    setRows(next)
    setIssues([])
  }

  function preview(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    const input: DocumentationRootInput[] = []
    const seen = new Set<string>()
    const invalid: typeof issues = []
    rows.forEach(({ fields }, index) => {
      const { audience, profile_id, path_prefix } = fields
      const area_id = fields.area_id.toLowerCase()
      const path = '/data/' + String(index)
      const identity = JSON.stringify([area_id, path_prefix])
      if (seen.has(identity))
        invalid.push({
          field: path + '/path_prefix',
          message: 'Этот путь в репозитории уже добавлен.',
        })
      seen.add(identity)
      if (profile_id !== gitProfile)
        invalid.push({
          field: path + '/profile_id',
          message: 'Выберите поддерживаемый профиль.',
        })
      if (
        audience !== 'team' &&
        audience !== 'organization' &&
        audience !== 'external'
      ) {
        invalid.push({
          field: path + '/audience',
          message: 'Выберите аудиторию.',
        })
        return
      }
      input.push({
        ...fields,
        area_id,
        owner_person_id: fields.owner_person_id?.toLowerCase() ?? null,
        audience,
      })
    })
    setIssues(invalid)
    if (!invalid.length)
      setProposal(rootProposal(input, baseline, repositories))
  }

  useEffect(() => {
    if (saved) onSaved()
  }, [saved, onSaved])

  return (
    <>
      <form ref={form} className="cp-doc-roots__form" onSubmit={preview}>
        {issues.length > 0 && (
          <div role="alert" tabIndex={-1} ref={errors}>
            <p>Не удалось сохранить корни</p>
            <ul>
              {issues.map(({ field, message }, index) => {
                const target = validationTarget(field)
                return (
                  <li key={index}>
                    {target ? <a href={'#' + target}>{message}</a> : message}
                  </li>
                )
              })}
            </ul>
          </div>
        )}
        <datalist id="documentation-repositories">
          {repositories &&
            [...repositories].map(([id, name]) => (
              <option key={id} value={id} label={name} />
            ))}
        </datalist>
        <datalist id="documentation-owners">
          {[...owners].map(([id, name]) => (
            <option key={id} value={id} label={name} />
          ))}
        </datalist>
        {!rows.length && <p>Корни документации не подключены.</p>}
        {rows.map((row, index) => (
          <RootFields
            key={row.key}
            index={index}
            fields={row.fields}
            issues={
              new Set(issues.map(({ field }) => validationTarget(field) ?? ''))
            }
            edit={(field, value) => {
              edit(
                rows.map((item) =>
                  item.key === row.key
                    ? {
                        ...row,
                        fields: {
                          ...row.fields,
                          [field]:
                            value || (field === 'owner_person_id' ? null : ''),
                        },
                      }
                    : item,
                ),
              )
            }}
            remove={() => {
              edit(rows.filter((item) => item.key !== row.key))
            }}
          />
        ))}
        <div className="cp-doc-roots__actions">
          <Button
            type="button"
            size="l"
            disabled={rows.length >= 100}
            onClick={() => {
              edit([...rows, newRow()])
            }}
          >
            Добавить корень
          </Button>
          <Button
            type="submit"
            size="l"
            view="action"
            ref={submitButton}
            disabled={!dirty}
          >
            Сохранить набор
          </Button>
        </div>
      </form>
      <UnsavedChangesDialog
        formName="Корни документации"
        formRef={form}
        dirty={dirty}
        confirmationOpen={Boolean(proposal)}
        onDiscard={() => {
          edit(toRows(baseline.data))
        }}
      />
      {proposal && (
        <ChangeConfirmationDialog
          title="Подтверждение корней документации"
          scope={serviceName}
          consequences="Сервис будет использовать показанный набор корней. Изменения вступят в силу после сохранения набора."
          dangerous={proposal.changes.some(({ label }) =>
            label.startsWith('Удаление'),
          )}
          openerRef={submitButton}
          proposal={proposal}
          submit={(attempt, signal) =>
            replaceDocumentationRoots(
              api,
              serviceId,
              attempt.draft,
              attempt.etag,
              attempt.idempotencyKey,
              signal,
            )
          }
          recheck={async (attempt, reason, signal) => {
            const latest = await readDocumentationRoots(api, serviceId, signal)
            signal.throwIfAborted()
            if (!isStrongETag(latest.etag)) return { kind: 'unknown' }
            if (reason === 'uncertain') {
              // The server checks the same idempotency key before If-Match.
              return { kind: 'retryable' }
            }
            if (latest.etag === attempt.etag) return { kind: 'unknown' }
            setBaseline(latest)
            return {
              kind: 'not-applied',
              proposal: rootProposal(attempt.draft, latest, repositories),
            }
          }}
          onSaved={(result) => {
            setBaseline(result)
            setRows(toRows(result.data))
            setSaved(true)
            void cache.invalidateQueries({
              queryKey: listDocumentationRoots(api, serviceId).queryKey,
            })
            void cache.invalidateQueries({
              queryKey: serviceDocumentPages(api, serviceId).queryKey,
            })
          }}
          onValidation={(validation) => {
            setIssues(validationIssues(validation))
          }}
          onClose={() => {
            setProposal(undefined)
          }}
        />
      )}
    </>
  )
}
