import { describeExceptionScope } from '@/entities/exception'
import { technicalLabels } from '@/shared/config'
import {
  decisionTypes,
  fieldNames,
  maxReasonLength,
} from '../model/exception-draft'
import type {
  ExceptionFields as Fields,
  FormIssues,
  ScopeOption,
} from '../model/exception-draft'

export function ExceptionFields({
  fields,
  scopes,
  issues,
  edit,
}: {
  fields: Fields
  scopes: ScopeOption[]
  issues: FormIssues
  edit: (patch: Partial<Fields>) => void
}) {
  const selected = scopes[fields.scopeIndex]

  function invalid(field: keyof typeof fieldNames) {
    return {
      'aria-invalid': Boolean(issues[field]),
      'aria-describedby': issues[field] ? 'exception-issues' : undefined,
    }
  }

  return (
    <>
      <label>
        {fieldNames.decision_type}
        <select
          id="decision_type"
          value={fields.type}
          {...invalid('decision_type')}
          onChange={(event) => {
            edit({ type: event.target.value })
          }}
        >
          <option value="">Выберите тип</option>
          {decisionTypes.map((type) => (
            <option key={type} value={type}>
              {technicalLabels.exceptionDecisionType[type]}
            </option>
          ))}
        </select>
      </label>
      {fields.type === 'accepted_risk' && (
        <p>Находка остаётся в истории. Принятие риска не исправляет её.</p>
      )}
      <label>
        {fieldNames.reason}
        <textarea
          id="reason"
          value={fields.reason}
          rows={4}
          {...invalid('reason')}
          onChange={(event) => {
            edit({ reason: event.target.value })
          }}
        />
        <span>
          {Array.from(fields.reason).length} / {maxReasonLength}
        </span>
      </label>
      <label>
        {fieldNames.scope}
        <select
          id="scope"
          value={fields.scopeIndex}
          {...invalid('scope')}
          onChange={(event) => {
            edit({ scopeIndex: Number(event.target.value) })
          }}
        >
          {scopes.map(({ label }, index) => (
            <option key={label} value={index}>
              {label}
            </option>
          ))}
        </select>
      </label>
      {selected && <p>{describeExceptionScope(selected.value)}</p>}
      <p>Даты указаны в вашем часовом поясе.</p>
      <label>
        {fieldNames.effective_from}
        <input
          id="effective_from"
          type="datetime-local"
          value={fields.start}
          {...invalid('effective_from')}
          onChange={(event) => {
            edit({ start: event.target.value })
          }}
        />
      </label>
      <label>
        {fieldNames.expires_at}
        <input
          id="expires_at"
          type="datetime-local"
          value={fields.end}
          {...invalid('expires_at')}
          onChange={(event) => {
            edit({ end: event.target.value })
          }}
        />
      </label>
    </>
  )
}
