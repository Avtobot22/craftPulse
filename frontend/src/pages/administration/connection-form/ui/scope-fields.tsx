import { Button, Checkbox, Select } from '@gravity-ui/uikit'
import { calendarValue, newScope, scopeLabels } from '../model/connection-draft'
import type { ScopeRow } from '../model/connection-draft'
import { fieldId } from '../model/connection-issues'
import type { Issue } from '../model/connection-issues'
import { Field, InputField } from './connection-fields'

function ScopeFields({
  index,
  row,
  kinds,
  issues,
  edit,
  remove,
  disabled,
}: {
  index: number
  row: ScopeRow
  kinds: (keyof typeof scopeLabels)[]
  issues: readonly Issue[]
  edit: (next: ScopeRow) => void
  remove: () => void
  disabled: boolean
}) {
  const path = '/scopes/' + String(index)
  const rowIssues = issues.filter(({ field }) => field === path)
  return (
    <fieldset id={fieldId(path)} tabIndex={-1} className="cp-connection__scope">
      <legend>Область {index + 1}</legend>
      {rowIssues.map(({ message }, i) => (
        <p key={i} className="cp-connection__error">
          {message}
        </p>
      ))}
      <div className="cp-connection__columns">
        <Field path={path + '/kind'} label="Вид области" issues={issues}>
          {(id, error) => (
            <Select
              id={id}
              size="xl"
              width="max"
              value={row.kind ? [row.kind] : []}
              placeholder="Выберите вид области"
              options={kinds.map((value) => ({
                value,
                content: scopeLabels[value],
              }))}
              onUpdate={([kind]) => {
                if (kind && kind !== row.kind) {
                  edit({
                    key: row.key,
                    kind,
                    native_scope_id: row.native_scope_id,
                    history_from: row.history_from,
                  })
                }
              }}
              disabled={disabled}
              {...(error
                ? { 'aria-describedby': error, validationState: 'invalid' }
                : {})}
            />
          )}
        </Field>
        <InputField
          path={path + '/native_scope_id'}
          label="Идентификатор области в источнике"
          issues={issues}
          value={row.native_scope_id}
          change={(native_scope_id) => {
            if (native_scope_id === row.native_scope_id) return
            edit({ ...row, displayName: '', native_scope_id })
          }}
          maxLength={255}
          disabled={disabled}
        />
      </div>
      {row.displayName && row.displayName !== row.native_scope_id && (
        <p>{row.displayName}</p>
      )}
      <Field
        path={path + '/history_from'}
        label="Начало истории (UTC)"
        issues={issues}
      >
        {(id, error) => (
          <>
            <input
              className="cp-connection__calendar"
              id={id}
              type="datetime-local"
              step="0.001"
              required={row.history_from !== null}
              disabled={disabled || row.history_from === null}
              value={calendarValue(row.history_from)}
              aria-invalid={Boolean(error)}
              aria-describedby={error}
              onChange={(event) => {
                const value = event.currentTarget.value
                const timestamp = Date.parse(value + 'Z')
                edit({
                  ...row,
                  history_from: Number.isFinite(timestamp)
                    ? new Date(timestamp).toISOString()
                    : '',
                })
              }}
            />
            <Checkbox
              disabled={disabled}
              size="l"
              checked={row.history_from === null}
              onUpdate={(checked) => {
                edit({ ...row, history_from: checked ? null : '' })
              }}
            >
              Вся доступная история
            </Checkbox>
          </>
        )}
      </Field>
      <Button type="button" size="l" onClick={remove} disabled={disabled}>
        Удалить область {index + 1}
      </Button>
    </fieldset>
  )
}

export function ScopeSection({
  rows,
  kinds,
  issues,
  disabled,
  change,
}: {
  rows: ScopeRow[]
  kinds: (keyof typeof scopeLabels)[]
  issues: readonly Issue[]
  disabled: boolean
  change: (rows: ScopeRow[]) => void
}) {
  const id = fieldId('/scopes')
  const groupIssues = issues.filter(({ field }) => field === '/scopes')
  return (
    <fieldset
      id={id}
      tabIndex={-1}
      aria-describedby={groupIssues.length ? id + '-error' : undefined}
      className="cp-connection__section"
    >
      <legend>Область и история</legend>
      {groupIssues.length > 0 && (
        <p id={id + '-error'} className="cp-connection__error">
          {groupIssues.map(({ message }) => message).join('. ')}
        </p>
      )}
      {rows.map((row, index) => (
        <ScopeFields
          key={row.key}
          row={row}
          index={index}
          kinds={kinds}
          issues={issues}
          edit={(next) => {
            change(rows.map((item) => (item.key === row.key ? next : item)))
          }}
          remove={() => {
            change(rows.filter((item) => item.key !== row.key))
          }}
          disabled={disabled}
        />
      ))}
      <div>
        <Button
          type="button"
          size="l"
          disabled={disabled || !kinds.length || rows.length >= 500}
          onClick={() => {
            change([...rows, newScope(kinds[0])])
          }}
        >
          Добавить область
        </Button>
      </div>
    </fieldset>
  )
}
