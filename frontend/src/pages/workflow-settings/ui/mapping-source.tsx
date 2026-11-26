import { Text } from '@gravity-ui/uikit'
import type { CatalogInstance } from '../api/workflow-responses'
import {
  newRule,
  resolutionMeanings,
  sourceLabel,
  statusMeanings,
  suggestedMeaning,
} from '../model/drafts'
import { fieldProps } from './form-errors'
import {
  BlockedFlag,
  mappingScope,
  replaceRule,
  RuleField,
  ValueRules,
} from './mapping-fields'
import type { SourceRules } from './mapping-fields'

export function MappingSource({
  instance,
  rules,
  issues,
  onChange,
  proposed,
  teams,
}: SourceRules & {
  instance: CatalogInstance
  proposed: boolean
  teams: readonly { id: string; name: string }[]
}) {
  const groups = [
    {
      kind: 'status',
      title: 'Статусы',
      entries: instance.statuses,
      meanings: statusMeanings,
    },
    {
      kind: 'resolution',
      title: 'Исходы',
      entries: instance.resolutions,
      meanings: resolutionMeanings,
    },
    {
      kind: 'issue_type',
      title: 'Типы задач',
      entries: instance.issue_types,
      meanings: null,
    },
  ] as const
  const props = { instance, rules, issues, onChange }

  return (
    <section className="cp-workflow__source">
      <Text as="h3" variant="subheader-2">
        {sourceLabel(instance)}
      </Text>
      {groups.map(({ kind, title, entries, meanings }) => (
        <fieldset key={kind}>
          <legend>{title}</legend>
          {!entries.length && <p>Нет значений в справочнике</p>}
          {entries.map((entry) => {
            const index = rules.findIndex(
              (rule) =>
                rule.instance_id === instance.instance_id &&
                rule.rule_kind === kind &&
                rule.native_ref === entry.native_id,
            )
            const current = rules[index]
            const path = `/rules/${String(index)}/meaning`
            const input = current ? fieldProps(mappingScope, issues, path) : {}
            const label = `${entry.name} (${entry.native_id})`
            const category = instance.statuses.find(
              (status) => status.native_id === entry.native_id,
            )?.source_category

            function update(value: string) {
              onChange(
                replaceRule(
                  rules,
                  index,
                  value
                    ? {
                        ...(current ??
                          newRule(kind, instance.instance_id, entry.native_id)),
                        meaning: value,
                      }
                    : undefined,
                ),
              )
            }

            return (
              <div className="cp-workflow__pair" key={entry.native_id}>
                <div>
                  {label}
                  {kind === 'status' && (
                    <small>
                      Категория источника · {category ?? 'не указана'}
                    </small>
                  )}
                </div>
                <div>
                  {meanings ? (
                    <select
                      {...input}
                      aria-label={`${kind === 'status' ? 'Смысл статуса' : 'Смысл исхода'} ${label}`}
                      value={current?.meaning ?? ''}
                      onChange={(event) => {
                        update(event.target.value)
                      }}
                    >
                      <option value="">Без правила</option>
                      {Object.entries(meanings).map(([code, name]) => (
                        <option key={code} value={code}>
                          {name}
                        </option>
                      ))}
                      {current && !Object.hasOwn(meanings, current.meaning) && (
                        <option value={current.meaning}>
                          {current.meaning}
                        </option>
                      )}
                    </select>
                  ) : (
                    <input
                      {...input}
                      aria-label={`Метка типа ${label}`}
                      value={current?.meaning ?? ''}
                      placeholder="Без правила"
                      maxLength={200}
                      onChange={(event) => {
                        update(event.target.value)
                      }}
                    />
                  )}
                  {kind === 'status' &&
                    current &&
                    proposed &&
                    current.meaning === suggestedMeaning(category) && (
                      <small>предложено</small>
                    )}
                  <RuleField issues={issues} path={path} />
                </div>
              </div>
            )
          })}
        </fieldset>
      ))}
      <ValueRules {...props} mode="size" teams={teams} />
      <ValueRules {...props} mode="team" teams={teams} />
      <BlockedFlag {...props} />
    </section>
  )
}
