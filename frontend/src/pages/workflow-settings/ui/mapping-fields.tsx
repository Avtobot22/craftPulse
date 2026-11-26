import { Button } from '@gravity-ui/uikit'
import type { CatalogInstance, MappingRule } from '../api/workflow-responses'
import { newRule } from '../model/drafts'
import type { FieldIssues } from '../model/validation'
import { fieldProps, FieldMessage } from './form-errors'

export const mappingScope = 'workflow-mapping'

export interface SourceRules {
  rules: MappingRule[]
  issues: FieldIssues
  onChange: (rules: MappingRule[]) => void
}

export function replaceRule(
  rules: MappingRule[],
  index: number,
  next?: MappingRule,
) {
  if (index < 0) return next ? [...rules, next] : rules
  return next
    ? rules.map((rule, at) => (at === index ? next : rule))
    : rules.filter((_, at) => at !== index)
}

export function RuleField({
  issues,
  path,
}: {
  issues: FieldIssues
  path: string
}) {
  return <FieldMessage scope={mappingScope} issues={issues} path={path} />
}

function FieldOptions({
  instance,
  value,
}: {
  instance: CatalogInstance
  value: string
}) {
  return (
    <>
      <option value="">Не выбрано</option>
      {value && !instance.fields.some((field) => field.native_id === value) && (
        <option value={value}>{value} (нет в справочнике)</option>
      )}
      {instance.fields.map((field) => (
        <option key={field.native_id} value={field.native_id}>
          {field.name} ({field.native_id}, {field.value_type})
        </option>
      ))}
    </>
  )
}

export function ValueRules({
  instance,
  rules,
  issues,
  onChange,
  mode,
  teams,
}: SourceRules & {
  instance: CatalogInstance
  mode: 'size' | 'team'
  teams: readonly { id: string; name: string }[]
}) {
  const fieldKind = mode === 'size' ? 'size_field' : 'team_field'
  const valueKind = mode === 'size' ? 'size_value' : 'team_value'
  const fieldIndex = rules.findIndex(
    (rule) =>
      rule.instance_id === instance.instance_id && rule.rule_kind === fieldKind,
  )
  const fieldRef = rules[fieldIndex]?.native_ref ?? ''
  const fieldPath = `/rules/${String(fieldIndex)}/native_ref`
  const ownValues = rules
    .map((rule, index) => ({ rule, index }))
    .filter(
      ({ rule }) =>
        rule.instance_id === instance.instance_id &&
        rule.rule_kind === valueKind,
    )
  const title = mode === 'size' ? 'размера' : 'команды'

  function selectField(ref: string) {
    const next = replaceRule(
      rules,
      fieldIndex,
      ref ? newRule(fieldKind, instance.instance_id, ref) : undefined,
    )
    onChange(
      next.filter(
        (rule) =>
          rule.instance_id !== instance.instance_id ||
          rule.rule_kind !== valueKind ||
          rule.native_ref === ref,
      ),
    )
  }

  return (
    <fieldset>
      <legend>{mode === 'size' ? 'Размер' : 'Команда'}</legend>
      <label className="cp-workflow__field">
        Поле {title}
        <select
          {...(fieldIndex < 0
            ? {}
            : fieldProps(mappingScope, issues, fieldPath))}
          value={fieldRef}
          onChange={(event) => {
            selectField(event.target.value)
          }}
        >
          <FieldOptions instance={instance} value={fieldRef} />
        </select>
        <RuleField issues={issues} path={fieldPath} />
      </label>
      {!!ownValues.length && (
        <div className="cp-workflow__rules">
          {ownValues.map(({ rule, index }, row) => {
            const path = `/rules/${String(index)}`
            const change = (patch: Partial<MappingRule>) => {
              onChange(replaceRule(rules, index, { ...rule, ...patch }))
            }
            return (
              <div className="cp-workflow__row" key={index}>
                <label className="cp-workflow__field">
                  Значение {title} в источнике {row + 1}
                  <input
                    {...fieldProps(
                      mappingScope,
                      issues,
                      path + '/native_value',
                    )}
                    value={rule.native_value}
                    maxLength={255}
                    onChange={(event) => {
                      change({ native_value: event.target.value })
                    }}
                  />
                  <RuleField issues={issues} path={path + '/native_value'} />
                </label>
                {mode === 'size' ? (
                  <>
                    <label className="cp-workflow__field">
                      Метка размера {row + 1}
                      <input
                        {...fieldProps(mappingScope, issues, path + '/meaning')}
                        value={rule.meaning}
                        maxLength={200}
                        onChange={(event) => {
                          change({ meaning: event.target.value })
                        }}
                      />
                      <RuleField issues={issues} path={path + '/meaning'} />
                    </label>
                    <label className="cp-workflow__field">
                      Порядок размера {row + 1}
                      <input
                        {...fieldProps(mappingScope, issues, path + '/rank')}
                        type="number"
                        min={0}
                        max={32767}
                        step={1}
                        value={rule.rank ?? ''}
                        onChange={(event) => {
                          change({
                            rank: Number.isFinite(event.target.valueAsNumber)
                              ? event.target.valueAsNumber
                              : null,
                          })
                        }}
                      />
                      <RuleField issues={issues} path={path + '/rank'} />
                    </label>
                  </>
                ) : (
                  <label className="cp-workflow__field">
                    Команда для значения {row + 1}
                    <select
                      {...fieldProps(mappingScope, issues, path + '/meaning')}
                      value={rule.meaning}
                      onChange={(event) => {
                        change({ meaning: event.target.value })
                      }}
                    >
                      <option value="">Выберите команду</option>
                      {rule.meaning &&
                        !teams.some((team) => team.id === rule.meaning) && (
                          <option value={rule.meaning}>{rule.meaning}</option>
                        )}
                      {teams.map((team) => (
                        <option key={team.id} value={team.id}>
                          {team.name}
                        </option>
                      ))}
                    </select>
                    <RuleField issues={issues} path={path + '/meaning'} />
                  </label>
                )}
                <Button
                  type="button"
                  view="outlined"
                  onClick={() => {
                    onChange(replaceRule(rules, index))
                  }}
                >
                  Удалить значение {title} {row + 1}
                </Button>
              </div>
            )
          })}
        </div>
      )}
      <Button
        type="button"
        view="outlined"
        disabled={!fieldRef || rules.length >= 2000}
        onClick={() => {
          onChange([
            ...rules,
            newRule(valueKind, instance.instance_id, fieldRef),
          ])
        }}
      >
        Добавить значение {title}
      </Button>
    </fieldset>
  )
}

export function BlockedFlag({
  instance,
  rules,
  issues,
  onChange,
}: SourceRules & { instance: CatalogInstance }) {
  const index = rules.findIndex(
    (rule) =>
      rule.instance_id === instance.instance_id &&
      rule.rule_kind === 'blocked_flag',
  )
  const current = rules[index]
  const value = current?.native_ref ?? ''
  const path = `/rules/${String(index)}`

  return (
    <fieldset>
      <legend>Флаг блокировки</legend>
      <div className="cp-workflow__row">
        <label className="cp-workflow__field">
          Поле флага блокировки
          <select
            {...(current
              ? fieldProps(mappingScope, issues, path + '/native_ref')
              : {})}
            value={value}
            onChange={(event) => {
              const ref = event.target.value
              onChange(
                replaceRule(
                  rules,
                  index,
                  ref
                    ? newRule(
                        'blocked_flag',
                        instance.instance_id,
                        ref,
                        'blocked',
                      )
                    : undefined,
                ),
              )
            }}
          >
            <FieldOptions instance={instance} value={value} />
          </select>
          <RuleField issues={issues} path={path + '/native_ref'} />
        </label>
        {current && (
          <label className="cp-workflow__field">
            Значение флага блокировки
            <input
              {...fieldProps(mappingScope, issues, path + '/native_value')}
              value={current.native_value}
              maxLength={255}
              onChange={(event) => {
                onChange(
                  replaceRule(rules, index, {
                    ...current,
                    native_value: event.target.value,
                  }),
                )
              }}
            />
            <RuleField issues={issues} path={path + '/native_value'} />
          </label>
        )}
      </div>
    </fieldset>
  )
}
