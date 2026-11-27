import { Button, TextInput } from '@gravity-ui/uikit'
import type { ReactNode } from 'react'
import type { Team } from '@/shared/api'
import { adminLabels } from '@/shared/config'
import {
  fileAccept,
  isTaskSnapshot,
  instanceKeyLimit,
  reportFormats,
  reportLabels,
  schemaLabels,
  versionLimit,
} from '../model/import-draft'
import type { ImportDraft, ImportField } from '../model/import-draft'
import { fieldLabels } from '../model/import-issues'
import type { ImportIssue } from '../model/import-issues'

export const fieldId = (field: ImportField) => 'import-' + field

interface FieldProps {
  field: ImportField
  issues: readonly ImportIssue[]
  hint?: string
  children: (
    id: string,
    invalid: boolean,
    description: string | undefined,
  ) => ReactNode
}

function Field({ field, issues, hint, children }: FieldProps) {
  const id = fieldId(field)
  const messages = issues
    .filter((issue) => issue.field === field)
    .map(({ message }) => message)
  const invalid = messages.length > 0
  const description = [hint && id + '-hint', invalid && id + '-error']
    .filter(Boolean)
    .join(' ')
  return (
    <div className="cp-import__field">
      <label htmlFor={id}>{fieldLabels[field]}</label>
      {children(id, invalid, description || undefined)}
      {hint && (
        <p id={id + '-hint'} className="cp-import__hint">
          {hint}
        </p>
      )}
      {invalid && (
        <p
          id={id + '-error'}
          className="cp-import__error"
          aria-live={field === 'file' ? 'polite' : undefined}
        >
          {messages.join('. ')}
        </p>
      )}
    </div>
  )
}

interface ImportFieldsProps {
  draft: ImportDraft
  file: File | undefined
  projects: readonly Team[]
  issues: readonly ImportIssue[]
  onEdit: <K extends keyof ImportDraft>(field: K, value: ImportDraft[K]) => void
  onFile: (file: File | undefined) => void
}

export function ImportFields({
  draft,
  file,
  projects,
  issues,
  onEdit,
  onFile,
}: ImportFieldsProps) {
  function select(
    field: keyof Omit<ImportDraft, 'snapshot'>,
    placeholder: string,
    options: [string, string][],
  ) {
    return (
      <Field field={field} issues={issues}>
        {(id, invalid, description) => (
          <select
            id={id}
            required
            value={draft[field]}
            aria-invalid={invalid}
            aria-describedby={description}
            onChange={(event) => {
              onEdit(field, event.currentTarget.value)
            }}
          >
            <option value="">{placeholder}</option>
            {options.map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </select>
        )}
      </Field>
    )
  }

  function input(
    field: 'schema_version' | 'source_instance_key' | 'commit_sha',
    maxLength: number,
    hint?: string,
  ) {
    return (
      <Field field={field} issues={issues} {...(hint ? { hint } : {})}>
        {(id, invalid, description) => (
          <TextInput
            id={id}
            size="xl"
            value={draft[field]}
            onUpdate={(value) => {
              onEdit(field, value)
            }}
            {...(invalid ? { validationState: 'invalid' } : {})}
            controlProps={{
              required: true,
              maxLength,
              'aria-describedby': description,
              'aria-invalid': invalid,
            }}
          />
        )}
      </Field>
    )
  }

  const reports = reportFormats(draft.schema_id)
  const csv = isTaskSnapshot(file, draft)
  const fileHint = reports.length
    ? 'Выберите файл выбранного формата отчёта. До 25 MiB.'
    : 'JSON для всех видов данных, CSV для снимка задач. До 25 MiB.'

  return (
    <>
      {select(
        'project_id',
        'Выберите проект',
        projects.map(({ id, name }) => [id, name]),
      )}
      <div className="cp-import__row">
        {select(
          'schema_id',
          'Выберите вид данных',
          Object.entries(schemaLabels),
        )}
        {input('schema_version', versionLimit)}
      </div>
      <div className="cp-import__row">
        {select(
          'source_provider',
          'Выберите провайдер источника',
          Object.entries(adminLabels.provider),
        )}
        {input(
          'source_instance_key',
          instanceKeyLimit,
          'Если ключ совпадает с ключом подключённого экземпляра, объекты сохранят те же идентичности.',
        )}
      </div>
      {reports.length > 0 && (
        <div className="cp-import__row">
          {select(
            'report_format',
            'Выберите формат отчёта',
            reports.map((format) => [format, reportLabels[format]]),
          )}
          {input('commit_sha', 64, '40 или 64 шестнадцатеричных символа.')}
        </div>
      )}
      <Field field="file" issues={issues} hint={fileHint}>
        {(id, invalid, description) => (
          <>
            <input
              type="file"
              id={id}
              required
              accept={fileAccept(draft)}
              aria-invalid={invalid}
              aria-describedby={description}
              onChange={(event) => {
                onFile(event.currentTarget.files?.[0])
              }}
            />
            {file && (
              <div className="cp-import__file">
                <span>
                  {file.name} · {file.size.toLocaleString('ru-RU')} Б
                </span>
                <Button
                  size="xl"
                  view="outlined"
                  onClick={() => {
                    const control = document.getElementById(id)
                    if (control instanceof HTMLInputElement) {
                      control.value = ''
                      control.focus()
                    }
                    onFile(undefined)
                  }}
                >
                  Удалить файл
                </Button>
              </div>
            )}
          </>
        )}
      </Field>
      <div className="cp-import__field">
        <label className="cp-import__snapshot">
          <input
            id={fieldId('snapshot')}
            type="checkbox"
            checked={csv || draft.snapshot}
            aria-disabled={csv}
            aria-invalid={issues.some(({ field }) => field === 'snapshot')}
            aria-describedby={
              issues.some(({ field }) => field === 'snapshot')
                ? 'import-snapshot-error'
                : undefined
            }
            onChange={(event) => {
              if (!csv) onEdit('snapshot', event.currentTarget.checked)
            }}
          />
          Файл содержит только текущее состояние, прошлый WIP из него не
          строится
        </label>
        {issues.some(({ field }) => field === 'snapshot') && (
          <p id="import-snapshot-error" className="cp-import__error">
            {issues
              .filter(({ field }) => field === 'snapshot')
              .map(({ message }) => message)
              .join('. ')}
          </p>
        )}
      </div>
    </>
  )
}
