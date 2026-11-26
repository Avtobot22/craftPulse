import { Button } from '@gravity-ui/uikit'
import { useLayoutEffect, useRef } from 'react'
import type { SubmitEventHandler, RefObject } from 'react'
import { durationInputs } from '../model/target-settings'
import type {
  FormIssue,
  HistoricalBasis,
  TargetInput,
} from '../model/target-settings'

interface TargetFormProps {
  formRef: RefObject<HTMLFormElement | null>
  issues: FormIssue[]
  input: TargetInput | null
  historical: HistoricalBasis | null
  disabled: boolean
  onSubmit: SubmitEventHandler<HTMLFormElement>
  edit: (change: Partial<TargetInput>) => void
  refreshBaseline: () => void
}

export function TargetForm({
  formRef,
  issues,
  input,
  historical,
  disabled,
  onSubmit,
  edit,
  refreshBaseline,
}: TargetFormProps) {
  const summaryRef = useRef<HTMLDivElement>(null)
  useLayoutEffect(() => {
    if (issues.length) summaryRef.current?.focus()
  }, [issues])
  const durationError = issues.find(({ field }) => field === 'target-days')
  const basisError = issues.find(({ field }) => field === 'target-basis')
  return (
    <form ref={formRef} onSubmit={onSubmit} noValidate>
      {issues.length > 0 && (
        <div
          role="alert"
          className="cp-targets__errors"
          tabIndex={-1}
          ref={summaryRef}
        >
          <strong>Проверьте срок</strong>
          <ul>
            {issues.map(({ field, message }) => (
              <li key={message}>
                {field ? <a href={`#${field}`}>{message}</a> : message}
              </li>
            ))}
          </ul>
        </div>
      )}
      <fieldset>
        <legend>Новая версия</legend>
        <div className="cp-targets__fields">
          <label>
            Сутки
            <input
              id="target-days"
              inputMode="numeric"
              value={input?.days ?? ''}
              onChange={(event) => {
                edit({ days: event.target.value })
              }}
              aria-invalid={Boolean(durationError)}
              aria-describedby={
                durationError ? 'target-duration-error' : 'target-duration-help'
              }
            />
          </label>
          <label>
            Часы
            <input
              id="target-hours"
              inputMode="decimal"
              value={input?.hours ?? ''}
              onChange={(event) => {
                edit({ hours: event.target.value })
              }}
              aria-invalid={Boolean(durationError)}
              aria-describedby={
                durationError ? 'target-duration-error' : 'target-duration-help'
              }
            />
          </label>
        </div>
        <p id="target-duration-help">
          1 сутки = 24 часа. Дробные часы округляются до целой секунды.
        </p>
        {durationError && (
          <p id="target-duration-error" className="cp-targets__errors">
            {durationError.message}
          </p>
        )}
        <label htmlFor="target-basis">Основание</label>
        <select
          id="target-basis"
          value={input?.historical ? 'historical_p85' : 'manual'}
          aria-invalid={Boolean(basisError)}
          aria-describedby={
            basisError ? 'target-basis-error' : 'target-basis-help'
          }
          onChange={(event) => {
            if (event.target.value === 'manual') edit({ historical: null })
            else if (historical)
              edit({ ...durationInputs(historical.seconds), historical })
          }}
        >
          <option value="manual">Вручную</option>
          <option value="historical_p85" disabled={!historical}>
            Исторический p85
          </option>
        </select>
        <p id="target-basis-help">
          {input?.historical?.lowSample || historical?.lowSample
            ? 'Малая выборка. Проверьте выбранный срок перед сохранением.'
            : !historical
              ? 'Для этой группы исторический p85 недоступен.'
              : 'При выборе p85 срок заполняется по ориентиру.'}
        </p>
        {basisError && (
          <div id="target-basis-error" className="cp-targets__errors">
            <p>{basisError.message}</p>
            <Button
              size="xl"
              onClick={() => {
                edit({ historical: null })
                refreshBaseline()
              }}
            >
              Обновить ориентир
            </Button>
          </div>
        )}
        <Button size="xl" view="action" type="submit" disabled={disabled}>
          Сохранить срок
        </Button>
      </fieldset>
    </form>
  )
}
