import { Button, TextInput } from '@gravity-ui/uikit'
import { useRef } from 'react'
import type { ReactNode } from 'react'
import type { Choice } from '../api/structure'
import { fieldLabels, statuses } from '../model/structure-draft'
import type {
  Draft,
  LinkSection,
  NamedSection,
  Section,
} from '../model/structure-draft'
import type { Issue } from '../model/structure-errors'

export function fieldId(section: Section, field?: string) {
  return 'organization-' + section + (field ? '-' + field : '')
}

function Field({
  issue,
  children,
  field,
  section,
}: {
  issue: Issue[]
  children: (attributes: {
    id: string
    'aria-invalid': boolean
    'aria-describedby'?: string | undefined
  }) => ReactNode
  field: string
  section: Section
}) {
  const id = fieldId(section, field)
  const errors = issue.filter((error) => error.field === field)
  return (
    <div className="cp-org__field">
      <label htmlFor={id}>{fieldLabels[field]}</label>
      {children({
        id,
        'aria-invalid': errors.length > 0,
        'aria-describedby': errors.length ? id + '-error' : undefined,
      })}
      {errors.length > 0 && (
        <span id={id + '-error'} className="cp-org__error">
          {errors.map(({ message }) => message).join('. ')}
        </span>
      )}
    </div>
  )
}

interface Editor {
  issues: Issue[]
}

function SelectField({
  issues,
  field,
  section,
  value,
  options,
  onChange,
  unavailable,
}: Editor & {
  field: string
  section: Section
  value: string
  options: Choice[]
  onChange: (value: string) => void
  unavailable?: string | undefined
}) {
  return (
    <Field issue={issues} field={field} section={section}>
      {(attributes) => (
        <>
          <select
            {...attributes}
            value={value}
            onChange={(event) => {
              onChange(event.target.value)
            }}
            disabled={Boolean(unavailable)}
          >
            <option value="">Выберите значение</option>
            {value && !options.some(({ id }) => id === value) && (
              <option value={value}>{value} (недоступно)</option>
            )}
            {options.map(({ id, name }) => (
              <option key={id} value={id}>
                {name}
              </option>
            ))}
          </select>
          {unavailable && <span>{unavailable}</span>}
        </>
      )}
    </Field>
  )
}

export function NamedFields({
  issues,
  section,
  row,
  onChange,
}: Editor & {
  section: NamedSection
  row: Draft[NamedSection][number]
  onChange: (row: Draft[NamedSection][number]) => void
}) {
  return (
    <>
      <Field issue={issues} section={section} field="name">
        {({ id, ...attributes }) => (
          <TextInput
            id={id}
            value={row.name}
            controlProps={attributes}
            onUpdate={(name) => {
              onChange({ ...row, name })
            }}
          />
        )}
      </Field>
      <SelectField
        issues={issues}
        field="status"
        section={section}
        value={row.status}
        options={[
          { id: 'active', name: statuses.active },
          { id: 'archived', name: statuses.archived },
        ]}
        onChange={(status) => {
          if (status === 'active' || status === 'archived')
            onChange({ ...row, status })
        }}
      />
      {row.id === null && (
        <p>
          Сначала сохраните запись. После сохранения можно добавить связи и
          компоненты.
        </p>
      )}
    </>
  )
}

export function ComponentFields({
  issues,
  row,
  services,
  repositories,
  unavailable,
  onChange,
}: Editor & {
  row: Draft['components'][number]
  services: Choice[]
  repositories: Choice[]
  unavailable?: string | undefined
  onChange: (row: Draft['components'][number]) => void
}) {
  const section = 'components'
  const statusErrors = issues.filter(({ field }) => field === 'status')
  const statusId = fieldId(section, 'status')
  const statusRef = useRef<HTMLDivElement>(null)
  return (
    <>
      <SelectField
        issues={issues}
        field="service_id"
        section={section}
        options={services}
        value={row.service_id}
        onChange={(service_id) => {
          onChange({ ...row, service_id })
        }}
      />
      <SelectField
        issues={issues}
        field="repository_area_id"
        section={section}
        options={repositories}
        value={row.repository_area_id}
        unavailable={unavailable}
        onChange={(repository_area_id) => {
          onChange({ ...row, repository_area_id })
        }}
      />
      <Field issue={issues} section={section} field="path_prefix">
        {({ id, ...attributes }) => (
          <TextInput
            id={id}
            value={row.path_prefix}
            controlProps={attributes}
            placeholder="Весь репозиторий"
            onUpdate={(path_prefix) => {
              onChange({ ...row, path_prefix })
            }}
          />
        )}
      </Field>
      <div
        ref={statusRef}
        id={statusId}
        tabIndex={-1}
        role="group"
        aria-label="Статус компонента"
        aria-invalid={statusErrors.length > 0}
        aria-describedby={statusErrors.length ? statusId + '-error' : undefined}
      >
        <p>Статус: {statuses[row.status]}</p>
        {statusErrors.length > 0 && (
          <ul id={statusId + '-error'} className="cp-org__error">
            {statusErrors.map(({ message }, index) => (
              <li key={index}>{message}</li>
            ))}
          </ul>
        )}
        <div className="cp-org__bar">
          {row.status !== 'confirmed' && (
            <Button
              disabled={Boolean(unavailable)}
              onClick={() => {
                statusRef.current?.focus()
                onChange({ ...row, status: 'confirmed' })
              }}
            >
              Подтвердить компонент
            </Button>
          )}
          {row.status !== 'rejected' && row.id !== null && (
            <Button
              onClick={() => {
                statusRef.current?.focus()
                onChange({ ...row, status: 'rejected' })
              }}
            >
              Отклонить компонент
            </Button>
          )}
        </div>
      </div>
    </>
  )
}

export interface LinkField {
  field: string
  value: string
  choices: Choice[]
  onChange: (value: string) => void
  unavailable?: string | undefined
}

export function LinkFields({
  section,
  issues,
  fields,
}: Editor & { section: LinkSection; fields: LinkField[] }) {
  return fields.map(({ field, choices, ...props }) => (
    <SelectField
      key={field}
      section={section}
      issues={issues}
      field={field}
      options={choices}
      {...props}
    />
  ))
}
