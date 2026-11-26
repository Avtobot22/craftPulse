import { Button } from '@gravity-ui/uikit'
import type { CalendarDraft } from '../api/workflow-responses'
import { weekdays } from '../api/workflow-responses'
import { weekdayNames } from '../model/drafts'
import type { FieldIssues } from '../model/validation'
import { fieldProps, FieldMessage, FormErrors } from './form-errors'

const scope = 'workflow-calendar'
const labels: Readonly<Record<string, string>> = {
  '/timezone': 'Часовой пояс календаря',
  '/workdays': 'Рабочие дни',
  '/work_start': 'Начало рабочего дня',
  '/work_end': 'Конец рабочего дня',
  '/holidays': 'Праздники',
}

export function CalendarForm({
  draft,
  issues,
  onChange,
}: {
  draft: CalendarDraft
  issues: FieldIssues
  onChange: (draft: CalendarDraft) => void
}) {
  return (
    <>
      <FormErrors
        scope={scope}
        issues={issues}
        describe={(path) =>
          labels[path] ??
          (path.startsWith('/holidays/') ? 'Дата праздника' : 'Календарь')
        }
      />
      <p>Длительности отчёта считаются в календарных днях.</p>
      <label className="cp-workflow__field">
        Часовой пояс календаря (IANA)
        <input
          {...fieldProps(scope, issues, '/timezone')}
          value={draft.timezone}
          maxLength={64}
          onChange={(event) => {
            onChange({ ...draft, timezone: event.target.value })
          }}
          placeholder="Europe/Moscow"
        />
        <FieldMessage scope={scope} issues={issues} path="/timezone" />
      </label>
      <fieldset
        className="cp-workflow__days"
        {...fieldProps(scope, issues, '/workdays')}
      >
        <legend>Рабочие дни</legend>
        {weekdays.map((day) => (
          <label key={day}>
            <input
              type="checkbox"
              checked={draft.workdays.includes(day)}
              onChange={(event) => {
                onChange({
                  ...draft,
                  workdays: event.target.checked
                    ? [...draft.workdays, day]
                    : draft.workdays.filter((value) => value !== day),
                })
              }}
            />{' '}
            {weekdayNames[day]}
          </label>
        ))}
        <FieldMessage scope={scope} issues={issues} path="/workdays" />
      </fieldset>
      <div className="cp-workflow__row">
        {(['work_start', 'work_end'] as const).map((key) => (
          <label key={key} className="cp-workflow__field">
            {labels['/' + key]}
            <input
              {...fieldProps(scope, issues, '/' + key)}
              type="time"
              value={draft[key] ?? ''}
              onChange={(event) => {
                onChange({ ...draft, [key]: event.target.value || null })
              }}
            />
            <FieldMessage scope={scope} issues={issues} path={'/' + key} />
          </label>
        ))}
      </div>
      <fieldset {...fieldProps(scope, issues, '/holidays')}>
        <legend>Праздники</legend>
        {draft.holidays.map((day, index) => (
          <div className="cp-workflow__row" key={index}>
            <label className="cp-workflow__field">
              Дата праздника {index + 1}
              <input
                {...fieldProps(scope, issues, `/holidays/${String(index)}`)}
                type="date"
                value={day}
                onChange={(event) => {
                  onChange({
                    ...draft,
                    holidays: draft.holidays.map((value, at) =>
                      at === index ? event.target.value : value,
                    ),
                  })
                }}
              />
              <FieldMessage
                scope={scope}
                issues={issues}
                path={`/holidays/${String(index)}`}
              />
            </label>
            <Button
              type="button"
              view="outlined"
              onClick={() => {
                onChange({
                  ...draft,
                  holidays: draft.holidays.filter((_, at) => at !== index),
                })
              }}
            >
              Удалить праздник {index + 1}
            </Button>
          </div>
        ))}
        <FieldMessage scope={scope} issues={issues} path="/holidays" />
        <Button
          type="button"
          view="outlined"
          disabled={draft.holidays.length >= 400}
          onClick={() => {
            onChange({ ...draft, holidays: [...draft.holidays, ''] })
          }}
        >
          Добавить праздник
        </Button>
      </fieldset>
    </>
  )
}
