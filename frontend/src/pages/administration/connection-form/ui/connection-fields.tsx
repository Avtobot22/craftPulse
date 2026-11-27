import { Button, Select, TextInput } from '@gravity-ui/uikit'
import { Link } from 'react-router'
import { adminLabels, routePaths } from '@/shared/config'
import { StatusTime } from '@/shared/ui'
import type {
  Adapter,
  Connection,
  Credentials,
} from '../model/connection-draft'
import { credentialPath, fieldId } from '../model/connection-issues'
import type { Issue } from '../model/connection-issues'

export function Field({
  path,
  label,
  issues,
  children,
}: {
  path: string
  label: string
  issues: readonly Issue[]
  children: (id: string, error: string | undefined) => React.ReactNode
}) {
  const id = fieldId(path)
  const messages = issues
    .filter((issue) => issue.field === path)
    .map(({ message }) => message)
  return (
    <div className="cp-connection__field">
      <label htmlFor={id}>{label}</label>
      {children(id, messages.length ? id + '-error' : undefined)}
      {messages.length > 0 && (
        <p className="cp-connection__error" id={id + '-error'}>
          {messages.join('. ')}
        </p>
      )}
    </div>
  )
}

export function InputField({
  path,
  label,
  issues,
  value,
  change,
  maxLength,
  type = 'text',
  disabled = false,
  required = true,
}: {
  path: string
  label: string
  issues: readonly Issue[]
  value: string
  change: (value: string) => void
  maxLength: number
  type?: 'text' | 'url' | 'password'
  disabled?: boolean
  required?: boolean
}) {
  return (
    <Field path={path} label={label} issues={issues}>
      {(id, error) => (
        <TextInput
          id={id}
          size="xl"
          type={type}
          disabled={disabled}
          value={value}
          onUpdate={change}
          autoComplete={type === 'password' ? 'new-password' : 'off'}
          {...(error ? { validationState: 'invalid' } : {})}
          controlProps={{
            maxLength,
            required,
            'aria-describedby': error,
          }}
        />
      )}
    </Field>
  )
}

export function SourceAdapter({
  adapters,
  adapter,
  connection,
  provider,
  select,
  issues,
  disabled,
}: {
  adapters: Adapter[]
  adapter: Adapter | undefined
  connection: Connection | undefined
  provider: string
  select: (provider: string) => void
  issues: readonly Issue[]
  disabled: boolean
}) {
  const dataKindLabels: Readonly<Record<string, string>> = adminLabels.dataKind
  return (
    <>
      <Field path="/provider" label="Адаптер" issues={issues}>
        {(id, error) => (
          <Select
            id={id}
            size="xl"
            width="max"
            value={provider ? [provider] : []}
            placeholder="Выберите адаптер"
            options={adapters.map(({ provider, display_name }) => ({
              value: provider,
              content: display_name,
            }))}
            onUpdate={([value]) => {
              if (value) select(value)
            }}
            disabled={Boolean(connection) || disabled}
            {...(error
              ? { 'aria-describedby': error, validationState: 'invalid' }
              : {})}
          />
        )}
      </Field>
      {adapter && (
        <dl className="cp-connection__details">
          {adapter.editions.length > 0 && (
            <>
              <dt>Поддерживаемые редакции</dt>
              <dd>{adapter.editions.join(', ')}</dd>
            </>
          )}
          <dt>Виды данных</dt>
          <dd>
            {adapter.data_kinds
              .map((kind) =>
                Object.hasOwn(dataKindLabels, kind)
                  ? dataKindLabels[kind]
                  : kind,
              )
              .join(', ')}
          </dd>
        </dl>
      )}
      {connection?.webhook_url && (
        <dl className="cp-connection__details">
          <dt>Адрес вебхука</dt>
          <dd>{connection.webhook_url}</dd>
        </dl>
      )}
      {!connection && (
        <p>
          Нет подходящего адаптера?{' '}
          <Link to={routePaths.importPackage}>Универсальный импорт</Link>
        </p>
      )}
    </>
  )
}

export function CredentialFields({
  adapter,
  connection,
  editing,
  values,
  issues,
  start,
  cancel,
  change,
  disabled,
}: {
  adapter: Adapter | undefined
  connection: Connection | undefined
  editing: boolean
  values: Credentials
  issues: readonly Issue[]
  start: () => void
  cancel: () => void
  change: (name: string, value: string) => void
  disabled: boolean
}) {
  const groupMessage = issues
    .filter(({ field }) => field === '/credentials')
    .map(({ message }) => message)
    .join('. ')
  return (
    <fieldset
      id={fieldId('/credentials')}
      tabIndex={-1}
      className="cp-connection__section"
      disabled={disabled}
    >
      <legend>Учётные данные</legend>
      {connection && (
        <div role="status">
          {connection.credentials_set_at ? (
            <>
              Учётные данные заданы.{' '}
              <StatusTime value={connection.credentials_set_at} />
            </>
          ) : (
            'Учётные данные не заданы'
          )}
        </div>
      )}
      {groupMessage && <p className="cp-connection__error">{groupMessage}</p>}
      {editing &&
        adapter?.credential_fields.map((field) => (
          <InputField
            key={field.name}
            path={credentialPath(field.name)}
            label={field.label}
            issues={issues}
            value={
              (Object.hasOwn(values, field.name)
                ? values[field.name]
                : undefined) ?? ''
            }
            change={(value) => {
              change(field.name, value)
            }}
            maxLength={8192}
            type={field.secret ? 'password' : 'text'}
            required={field.required}
            disabled={disabled}
          />
        ))}
      {connection && Boolean(adapter?.credential_fields.length) && (
        <div>
          <Button
            type="button"
            size="l"
            onClick={editing ? cancel : start}
            disabled={disabled}
          >
            {editing ? 'Отменить замену' : 'Заменить учётные данные'}
          </Button>
        </div>
      )}
    </fieldset>
  )
}
