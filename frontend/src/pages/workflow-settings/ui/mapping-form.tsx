import { Button } from '@gravity-ui/uikit'
import type {
  CalendarVersion,
  Catalog,
  MappingDraft,
} from '../api/workflow-responses'
import { ruleLabel } from '../model/drafts'
import type { FieldIssues } from '../model/validation'
import { fieldProps, FormErrors } from './form-errors'
import { mappingScope, replaceRule, RuleField } from './mapping-fields'
import { MappingSource } from './mapping-source'
import { UnmappedStatuses } from './unmapped-statuses'

export function MappingForm({
  draft,
  issues,
  catalog,
  calendars,
  proposed,
  teams,
  onChange,
}: {
  draft: MappingDraft
  issues: FieldIssues
  catalog: Catalog
  calendars: CalendarVersion[]
  proposed: boolean
  teams: readonly { id: string; name: string }[]
  onChange: (draft: MappingDraft) => void
}) {
  const currentCalendar = draft.calendar_version_no
  const statuses = [
    ...catalog.unmapped_statuses,
    ...catalog.instances.flatMap((instance) =>
      instance.statuses.map((status) => ({
        instance_id: instance.instance_id,
        native_id: status.native_id,
      })),
    ),
  ].filter(
    (status) =>
      !draft.rules.some(
        (rule) =>
          rule.rule_kind === 'status' &&
          rule.instance_id === status.instance_id &&
          rule.native_ref === status.native_id,
      ),
  )
  const missing = draft.rules
    .map((rule, index) => ({ rule, index }))
    .filter(
      ({ rule }) =>
        !catalog.instances.some(
          (instance) => instance.instance_id === rule.instance_id,
        ),
    )

  function describe(path: string) {
    if (path === '/timezone') return 'Часовой пояс отчёта'
    if (path === '/calendar_version_no') return 'Версия рабочего календаря'
    const match = /^\/rules\/(\d+)/u.exec(path)
    const rule = match ? draft.rules[Number(match[1])] : undefined
    return rule ? ruleLabel(rule, catalog) : 'Правила сопоставления'
  }

  return (
    <>
      <FormErrors scope={mappingScope} issues={issues} describe={describe} />
      <div className="cp-workflow__row">
        <label className="cp-workflow__field">
          Часовой пояс отчёта (IANA)
          <input
            {...fieldProps(mappingScope, issues, '/timezone')}
            value={draft.timezone}
            maxLength={64}
            onChange={(event) => {
              onChange({ ...draft, timezone: event.target.value })
            }}
          />
          <RuleField issues={issues} path="/timezone" />
        </label>
        <label className="cp-workflow__field">
          Версия рабочего календаря
          <select
            {...fieldProps(mappingScope, issues, '/calendar_version_no')}
            value={currentCalendar ?? ''}
            onChange={(event) => {
              const picked = calendars.find(
                (version) => String(version.version_no) === event.target.value,
              )
              onChange({
                ...draft,
                calendar_version_no: picked?.version_no ?? null,
                timezone: picked?.timezone ?? draft.timezone,
              })
            }}
          >
            <option value="">Без рабочего календаря</option>
            {currentCalendar !== null &&
              !calendars.some(
                (version) => version.version_no === currentCalendar,
              ) && (
                <option value={currentCalendar}>
                  Версия {currentCalendar}
                </option>
              )}
            {calendars.map((version) => (
              <option key={version.version_no} value={version.version_no}>
                Версия {version.version_no} · {version.timezone}
              </option>
            ))}
          </select>
          <RuleField issues={issues} path="/calendar_version_no" />
        </label>
      </div>
      {proposed && (
        <p>
          Категории источника подсказывают смысл. Предложенные правила вступят в
          силу после подтверждения.
        </p>
      )}
      {catalog.instances.map((instance) => (
        <MappingSource
          key={instance.instance_id}
          instance={instance}
          rules={draft.rules}
          issues={issues}
          teams={teams}
          proposed={proposed}
          onChange={(rules) => {
            onChange({ ...draft, rules })
          }}
        />
      ))}
      <UnmappedStatuses statuses={statuses} catalog={catalog} />
      {!!missing.length && (
        <fieldset>
          <legend>Правила для недоступных экземпляров</legend>
          <ul>
            {missing.map(({ rule, index }) => (
              <li key={index}>
                {ruleLabel(rule, catalog)}{' '}
                <Button
                  type="button"
                  view="outlined"
                  onClick={() => {
                    onChange({
                      ...draft,
                      rules: replaceRule(draft.rules, index),
                    })
                  }}
                >
                  Удалить правило
                </Button>
              </li>
            ))}
          </ul>
        </fieldset>
      )}
    </>
  )
}
