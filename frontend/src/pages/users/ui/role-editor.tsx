import { Button } from '@gravity-ui/uikit'
import { useQueries, useQueryClient } from '@tanstack/react-query'
import { useLayoutEffect, useRef, useState } from 'react'
import { isStrongETag, isUnavailableError } from '@/shared/api'
import type { ApiClient, ApiResponse } from '@/shared/api'
import {
  ChangeConfirmationDialog,
  UnavailableRedirect,
  UnsavedChangesDialog,
} from '@/shared/ui'
import type {
  ChangeAttempt,
  ChangeProposal,
  ChangeRecheck,
  RecheckReason,
} from '@/shared/ui'
import { fetchRoles, putRoles, roleAreas, userRoles } from '../api/users-api'
import type { Assignment, RoleInput, User } from '../api/users-api'
import { roleIssues } from '../model/role-errors'
import {
  areaFields,
  describeRole,
  rebaseRoles,
  roleChanges,
  roleInput,
  roleKey,
  rolesChanged,
} from '../model/roles'
import type { AreaCatalog } from '../model/roles'
import { emptyRole, newAssignment, NewRoleFields } from './new-role-fields'
import { RoleTable } from './role-table'
import { userName } from './user-list'

type Snapshot = ApiResponse<Assignment[]>
const maxAssignments = 200

export function RoleEditor({
  api,
  user,
  initial,
  onSaved,
}: {
  api: ApiClient
  user: User
  initial: Snapshot
  onSaved: () => void
}) {
  const client = useQueryClient()
  const [snapshot, setSnapshot] = useState(initial)
  const [draft, setDraft] = useState(() => initial.data.map(roleInput))
  const [input, setInput] = useState(emptyRole)
  const [issues, setIssues] = useState(new Map<string, string>())
  const [proposal, setProposal] = useState<ChangeProposal<RoleInput[]>>()
  const [denial, setDenial] = useState<Error>()
  const [saved, setSaved] = useState(false)
  const formRef = useRef<HTMLFormElement>(null)
  const openerRef = useRef<HTMLButtonElement>(null)
  const roleRef = useRef<HTMLSelectElement>(null)
  const errorRef = useRef<HTMLDivElement>(null)

  const [teams, projects, services] = useQueries({
    queries: areaFields.map((field) => ({
      ...roleAreas(api, field),
      enabled:
        draft.some((item) => item[field] !== null) ||
        snapshot.data.some((item) => item[field] !== null),
    })),
  })
  const catalog: AreaCatalog = {}
  if (teams?.data) catalog.team_id = teams.data
  if (projects?.data) catalog.project_id = projects.data
  if (services?.data) catalog.service_id = services.data

  useLayoutEffect(() => {
    if (issues.size && !proposal) errorRef.current?.focus()
  }, [issues, proposal])

  const changed = rolesChanged(snapshot.data, draft)
  const versionAvailable = isStrongETag(snapshot.etag)

  function edit(next: RoleInput[]) {
    setDraft(next)
    setIssues(new Map())
    setSaved(false)
  }

  function remember(result: Snapshot) {
    setSnapshot(result)
    client.setQueryData(userRoles(api, user.id).queryKey, result)
  }

  function add() {
    const role = newAssignment(input)
    if (!role) return
    if (draft.some((item) => roleKey(item) === roleKey(role))) {
      setIssues(new Map([['/data', 'Такое назначение уже есть.']]))
      return
    }
    if (draft.length >= maxAssignments) {
      setIssues(new Map([['/data', 'Допустимо не более 200 назначений.']]))
      return
    }
    edit([...draft, role])
    setInput(emptyRole)
    roleRef.current?.focus()
  }

  async function recheck(
    attempt: ChangeAttempt<RoleInput[]>,
    reason: RecheckReason,
    signal: AbortSignal,
  ): Promise<ChangeRecheck<RoleInput[], Snapshot>> {
    let current: Snapshot
    try {
      current = await fetchRoles(api, user.id, signal)
    } catch (error) {
      if (isUnavailableError(error)) setDenial(error)
      throw error
    }
    if (!isStrongETag(current.etag)) return { kind: 'unknown' }
    if (reason === 'uncertain' && !rolesChanged(current.data, attempt.draft))
      return { kind: 'applied', result: current }
    if (current.etag === attempt.etag) return { kind: 'unknown' }

    const next = rebaseRoles(snapshot.data, attempt.draft, current.data).map(
      roleInput,
    )
    remember(current)
    edit(next)
    if (next.length > maxAssignments) {
      setIssues(new Map([['/data', 'Допустимо не более 200 назначений.']]))
      setProposal(undefined)
      return { kind: 'unknown' }
    }
    return {
      kind: 'not-applied',
      proposal: {
        etag: current.etag,
        baseline: {
          label: 'Актуальный набор ролей',
          value:
            current.data
              .map((item) => describeRole(item, catalog))
              .join('\n') || 'Нет назначений',
        },
        draft: next,
        changes: roleChanges(current.data, next, catalog),
      },
    }
  }

  function preview() {
    const etag = snapshot.etag
    if (!isStrongETag(etag) || draft.length > maxAssignments) return
    setIssues(new Map())
    setProposal({
      etag,
      draft,
      changes: roleChanges(snapshot.data, draft, catalog),
    })
  }

  if (denial) return <UnavailableRedirect error={denial} />

  return (
    <form
      ref={formRef}
      aria-labelledby="user-roles-title"
      className="cp-users__panel"
      onSubmit={(event) => {
        event.preventDefault()
        add()
      }}
    >
      <h3 id="user-roles-title">Роли и области</h3>
      <p>Роль не расширяет права в источниках.</p>
      <p>Версия набора ролей: {snapshot.etag ?? 'Недоступна'}</p>
      {!versionAvailable && (
        <p role="alert">
          Версия недоступна. Откройте карточку заново перед сохранением.
        </p>
      )}
      {saved && <p role="status">Роли сохранены</p>}
      {issues.size > 0 && (
        <div
          ref={errorRef}
          role="alert"
          tabIndex={-1}
          className="cp-users__error"
        >
          Проверьте назначения ролей.
          {[...issues]
            .filter(
              ([path]) =>
                !/^\/data\/\d+(\/(role|team_id|project_id|service_id))?$/u.test(
                  path,
                ),
            )
            .map(([path, message]) => (
              <p key={path}>{message}</p>
            ))}
        </div>
      )}
      <RoleTable
        draft={draft}
        assigned={
          new Map(
            snapshot.data.map((item) => [roleKey(item), item.assigned_at]),
          )
        }
        catalog={catalog}
        issues={issues}
        onRemove={(role) => {
          edit(draft.filter((item) => roleKey(item) !== roleKey(role)))
          roleRef.current?.focus()
        }}
      />
      <NewRoleFields
        api={api}
        value={input}
        roleRef={roleRef}
        onChange={setInput}
      />
      <div className="cp-users__controls">
        <Button size="xl" type="submit" disabled={!newAssignment(input)}>
          Добавить назначение
        </Button>
        <Button
          ref={openerRef}
          size="xl"
          view="action"
          disabled={
            !changed ||
            !versionAvailable ||
            Boolean(input.role) ||
            draft.length > maxAssignments
          }
          onClick={preview}
        >
          Просмотреть изменения ролей
        </Button>
      </div>
      {proposal && (
        <ChangeConfirmationDialog
          title="Подтвердите изменение ролей"
          scope={userName(user)}
          consequences="Новый набор ролей начнёт действовать со следующего запроса пользователя."
          proposal={proposal}
          openerRef={openerRef}
          wide
          submit={(attempt, signal) =>
            putRoles(api, user.id, attempt.draft, attempt.etag, signal)
          }
          recheck={recheck}
          onSaved={(result) => {
            remember(result)
            edit(result.data.map(roleInput))
            setSaved(true)
            onSaved()
          }}
          onValidation={(validation) => {
            setIssues(roleIssues(validation))
          }}
          onRejected={(error) => {
            if (!isUnavailableError(error)) return false
            setDenial(error)
            return true
          }}
          onClose={() => {
            setProposal(undefined)
          }}
        />
      )}
      <UnsavedChangesDialog
        formName="Роли и области"
        formRef={formRef}
        dirty={changed || Boolean(input.role)}
        confirmationOpen={Boolean(proposal)}
        onDiscard={() => {
          edit(snapshot.data.map(roleInput))
          setInput(emptyRole)
        }}
      />
    </form>
  )
}
