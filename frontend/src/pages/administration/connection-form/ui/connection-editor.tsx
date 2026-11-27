import { Button } from '@gravity-ui/uikit'
import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { SubmitEvent } from 'react'
import { generatePath, useNavigate } from 'react-router'
import { getConnection, isStrongETag } from '@/shared/api'
import type { ApiClient, ApiResponse } from '@/shared/api'
import { routePaths } from '@/shared/config'
import { UnsavedChangesDialog } from '@/shared/ui'
import { useSaveConnection } from '../api/use-save-connection'
import type { ConnectionWrite } from '../api/use-save-connection'
import {
  connectionChanges,
  connectionDraft,
  newScope,
  scopeInputs,
  scopeKinds,
} from '../model/connection-draft'
import type {
  Adapter,
  Connection,
  ConnectionDraft,
  Credentials,
} from '../model/connection-draft'
import { editablePaths, fieldId, formIssues } from '../model/connection-issues'
import type { Issue } from '../model/connection-issues'
import { ConnectionDialog } from './connection-dialog'
import {
  CredentialFields,
  InputField,
  SourceAdapter,
} from './connection-fields'
import { ScopeSection } from './scope-fields'

export function ConnectionEditor({
  api,
  adapters,
  initial,
}: {
  api: ApiClient
  adapters: Adapter[]
  initial: ApiResponse<Connection> | undefined
}) {
  const cache = useQueryClient()
  const navigate = useNavigate()
  const [emptyDraft] = useState(() => connectionDraft())
  const [baseline, setBaseline] = useState(initial)
  const [draft, setDraft] = useState(() => connectionDraft(initial?.data))
  const [credentials, setCredentials] = useState<Credentials>({})
  const [replacing, setReplacing] = useState(false)
  const [issues, setIssues] = useState<readonly Issue[]>([])
  const [dialog, setDialog] = useState<'draft' | 'conflict'>()
  const [reloading, setReloading] = useState(false)
  const [reloadFailed, setReloadFailed] = useState(false)
  const [createdId, setCreatedId] = useState<string>()
  const [saved, setSaved] = useState(false)
  const form = useRef<HTMLFormElement>(null)
  const summary = useRef<HTMLDivElement>(null)
  const submitButton = useRef<HTMLButtonElement>(null)
  const sending = useRef(false)
  const mutation = useSaveConnection(api)

  const connection = baseline?.data
  const adapter = adapters.find(({ provider }) => provider === draft.provider)
  const kinds = scopeKinds(adapter)
  const changes = connectionChanges(
    draft,
    connection ? connectionDraft(connection) : emptyDraft,
  )
  const pendingCredentials = Object.keys(credentials).length > 0
  const dirty =
    !createdId &&
    (Object.values(changes).some(Boolean) ||
      (!connection && Boolean(draft.provider)) ||
      pendingCredentials)
  const editingCredentials = !connection || replacing
  const busy = mutation.isPending || reloading
  const readonly = Boolean(
    connection && (connection.adapter_kind !== 'builtin' || !adapter),
  )

  useLayoutEffect(() => {
    if (issues.length) summary.current?.focus()
  }, [issues])

  useEffect(() => {
    if (createdId)
      void navigate(
        generatePath(routePaths.connectionDetail, { connectionId: createdId }),
        { replace: true },
      )
  }, [createdId, navigate])

  function edit(next: ConnectionDraft) {
    setDraft(next)
    setIssues([])
    setSaved(false)
  }

  function reset(response: ApiResponse<Connection> | undefined) {
    setBaseline(response)
    edit(connectionDraft(response?.data))
    setReplacing(false)
    setCredentials({})
  }

  function prepareWrite(): ConnectionWrite | undefined {
    const errors = formIssues(
      draft,
      adapter,
      editingCredentials ? credentials : undefined,
    )
    if (connection && replacing && !pendingCredentials && !errors.length)
      errors.push({
        field: '/credentials',
        message: 'Введите новые учётные данные',
      })
    setIssues(errors)
    if (errors.length || !adapter) return undefined

    const scopes = scopeInputs(draft.scopes, adapter)
    const fields = editablePaths(draft, adapter, editingCredentials)
    if (!baseline)
      return {
        kind: 'create',
        body: {
          provider: adapter.provider,
          name: draft.name,
          base_url: draft.base_url,
          scopes,
          adapter_kind: 'builtin',
          adapter_url: null,
          adapter_key: null,
        },
        fields,
      }
    if (!isStrongETag(baseline.etag)) {
      setDialog('conflict')
      return undefined
    }
    return {
      kind: 'update',
      id: baseline.data.id,
      etag: baseline.etag,
      body: {
        ...(changes.name ? { name: draft.name } : {}),
        ...(changes.base_url ? { base_url: draft.base_url } : {}),
        ...(changes.scopes ? { scopes } : {}),
      },
      fields,
    }
  }

  async function send(write: ConnectionWrite) {
    if (sending.current) return
    sending.current = true
    try {
      const result = await mutation.save(
        write,
        editingCredentials ? credentials : undefined,
      )
      if (result.kind === 'conflict') {
        if (write.kind === 'update') setDialog('conflict')
        else setIssues([{ message: 'Не удалось сохранить подключение' }])
        return
      }
      setDialog(undefined)
      if (result.kind === 'issues') {
        setIssues(result.issues)
        return
      }
      const { response } = result
      const queryKey = getConnection(api, response.data.id).queryKey
      // An older GET already in flight must not replace the saved snapshot.
      await cache.cancelQueries({ queryKey })
      cache.setQueryData(queryKey, response)
      void cache.invalidateQueries({ queryKey: ['connections', 'list'] })
      reset(response)
      setSaved(true)
      if (write.kind === 'create') setCreatedId(response.data.id)
    } catch {
      setDialog(undefined)
      setIssues([{ message: 'Не удалось сохранить подключение' }])
    } finally {
      setCredentials({})
      sending.current = false
    }
  }

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    if (busy || readonly || dialog || sending.current) return
    if (baseline && !dirty && !replacing) return
    const write = prepareWrite()
    if (!write) return
    if (
      connection?.status === 'active' &&
      (changes.base_url || changes.scopes)
    ) {
      setDialog('draft')
      return
    }
    void send(write)
  }

  async function reload() {
    if (!connection || sending.current) return
    sending.current = true
    setReloading(true)
    setReloadFailed(false)
    try {
      const latest = await cache.query({
        ...getConnection(api, connection.id),
        staleTime: 0,
      })
      if (!isStrongETag(latest.etag) || latest.etag === baseline.etag) {
        setReloadFailed(true)
        return
      }
      reset(latest)
      setDialog(undefined)
    } catch {
      setReloadFailed(true)
    } finally {
      sending.current = false
      setReloading(false)
    }
  }

  function accept() {
    if (dialog === 'conflict') {
      void reload()
      return
    }
    const write = prepareWrite()
    if (write) void send(write)
  }

  return (
    <>
      <form
        className="cp-connection__form"
        ref={form}
        onSubmit={submit}
        autoComplete="off"
        noValidate
      >
        {issues.length > 0 && (
          <div
            className="cp-connection__summary"
            ref={summary}
            role="alert"
            tabIndex={-1}
          >
            <strong>Не удалось сохранить подключение</strong>
            <ul>
              {issues.map(({ field, message }, index) => (
                <li key={index}>
                  {field ? (
                    <a
                      href={'#' + fieldId(field)}
                      onClick={(event) => {
                        event.preventDefault()
                        const target = document.getElementById(fieldId(field))
                        if (target?.matches(':disabled')) {
                          target
                            .closest('.cp-connection__field')
                            ?.querySelector<HTMLInputElement>(
                              'input:not(:disabled)',
                            )
                            ?.focus()
                        } else {
                          target?.focus()
                        }
                      }}
                    >
                      {message}
                    </a>
                  ) : (
                    message
                  )}
                </li>
              ))}
            </ul>
          </div>
        )}
        {readonly && (
          <p role="status">
            Встроенный адаптер этого подключения недоступен для редактирования.
          </p>
        )}
        {!connection && !adapters.length && (
          <p role="status">
            В этой установке нет доступных встроенных адаптеров.
          </p>
        )}
        <fieldset
          className="cp-connection__section"
          disabled={busy || readonly}
        >
          <legend>Источник</legend>
          <SourceAdapter
            adapters={adapters}
            adapter={adapter}
            connection={connection}
            provider={draft.provider}
            select={(provider) => {
              const nextKinds = scopeKinds(
                adapters.find((item) => item.provider === provider),
              )
              setCredentials({})
              edit({
                ...draft,
                provider,
                scopes: draft.scopes.length
                  ? draft.scopes.map((row) => ({
                      ...row,
                      kind:
                        nextKinds.find((kind) => kind === row.kind) ??
                        nextKinds[0] ??
                        '',
                    }))
                  : [newScope(nextKinds[0])],
              })
            }}
            issues={issues}
            disabled={busy || readonly}
          />
          <InputField
            path="/name"
            label="Имя подключения"
            issues={issues}
            value={draft.name}
            change={(name) => {
              edit({ ...draft, name })
            }}
            maxLength={200}
            disabled={busy || readonly}
          />
          <InputField
            path="/base_url"
            label="Базовый адрес источника"
            issues={issues}
            value={draft.base_url}
            change={(base_url) => {
              edit({ ...draft, base_url })
            }}
            maxLength={2048}
            type="url"
            disabled={busy || readonly}
          />
        </fieldset>
        <CredentialFields
          adapter={adapter}
          connection={connection}
          editing={editingCredentials}
          values={credentials}
          issues={issues}
          start={() => {
            setReplacing(true)
            setSaved(false)
          }}
          cancel={() => {
            setReplacing(false)
            setCredentials({})
            setIssues([])
          }}
          change={(name, value) => {
            setCredentials((previous) =>
              value
                ? { ...previous, [name]: value }
                : Object.fromEntries(
                    Object.entries(previous).filter(([key]) => key !== name),
                  ),
            )
            setIssues([])
          }}
          disabled={busy || readonly}
        />
        <ScopeSection
          rows={draft.scopes}
          kinds={kinds}
          issues={issues}
          disabled={busy || readonly}
          change={(scopes) => {
            edit({ ...draft, scopes })
          }}
        />
        <footer className="cp-connection__actions">
          <Button
            type="submit"
            size="xl"
            view="action"
            ref={submitButton}
            loading={mutation.isPending}
            disabled={
              busy ||
              readonly ||
              !adapters.length ||
              Boolean(connection && !dirty && !replacing)
            }
          >
            {connection?.status === 'active'
              ? 'Сохранить'
              : 'Сохранить черновик'}
          </Button>
          <Button
            type="button"
            size="xl"
            view="outlined"
            disabled={busy}
            onClick={() => {
              void navigate(routePaths.connections)
            }}
          >
            Отмена
          </Button>
          {saved && <span role="status">Подключение сохранено</span>}
        </footer>
      </form>
      <UnsavedChangesDialog
        formName="Настройки подключения"
        formRef={form}
        dirty={dirty}
        confirmationOpen={Boolean(dialog) || busy}
        hasSensitiveInput={pendingCredentials}
        onDiscard={() => {
          reset(baseline)
        }}
      />
      {dialog && (
        <ConnectionDialog
          key={dialog}
          kind={dialog}
          busy={busy}
          reloadFailed={reloadFailed}
          accept={accept}
          close={() => {
            setDialog(undefined)
            setReloadFailed(false)
          }}
          opener={submitButton}
        />
      )}
    </>
  )
}
