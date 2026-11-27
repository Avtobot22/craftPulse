import { Button, Checkbox, Label } from '@gravity-ui/uikit'
import { useRef, useState } from 'react'
import { contractLabel, providerLabel } from '@/shared/config'
import { StatusTime, TechnicalValue } from '@/shared/ui'
import {
  accountLabel,
  assignmentLabel,
  basisLabels,
  batchLimit,
  decisionLabel,
  isExactMatch,
  kindLabel,
  outcomeLabel,
  personName,
} from '../model/assignments'
import type {
  Account,
  Decision,
  Draft,
  Person,
  RowResult,
} from '../model/assignments'
import { PersonPicker } from './person-picker'

interface TableState {
  people: ReadonlyMap<string, Person>
  drafts: ReadonlyMap<string, Draft>
  results: ReadonlyMap<string, RowResult>
  refreshing: boolean
  decide: (account: Account, decision: Decision | null) => void
}

export function accountRowId(accountId: string) {
  return `source-account-${accountId}`
}

export function AccountsTable({
  accounts,
  label,
  state,
}: {
  accounts: readonly Account[]
  label: string
  state: TableState
}) {
  return (
    <div
      className="cp-accounts__scroll"
      role="region"
      aria-label={label}
      tabIndex={0}
    >
      <table className="cp-accounts__table" aria-label={label}>
        <thead>
          <tr>
            <th scope="col">Учётная запись</th>
            <th scope="col">Действующая привязка</th>
            <th scope="col">Предложения</th>
            <th scope="col">Решение</th>
          </tr>
        </thead>
        <tbody>
          {accounts.map((account) => (
            <AccountRow key={account.id} account={account} state={state} />
          ))}
        </tbody>
      </table>
    </div>
  )
}

function AccountRow({
  account,
  state,
}: {
  account: Account
  state: TableState
}) {
  const { people, drafts, results, refreshing, decide } = state
  const [editing, setEditing] = useState(false)
  const row = useRef<HTMLTableRowElement>(null)
  const draft = drafts.get(account.id)
  const result = results.get(account.id)
  const unavailable = result?.refresh === 'missing'
  const disabled =
    refreshing ||
    unavailable ||
    result?.refresh === 'needed' ||
    (!draft && drafts.size >= batchLimit)
  const hasProposedKind =
    account.proposed_kind === 'bot' || account.proposed_kind === 'system'
  const hasCandidates = account.proposals.length > 0 || hasProposedKind
  const name = accountLabel(account)

  function choose(decision: Decision) {
    decide(account, decision)
  }

  return (
    <tr ref={row} id={accountRowId(account.id)} tabIndex={-1}>
      <th scope="row">
        <div>{account.display_name ?? 'Имя не указано'}</div>
        <div className="cp-accounts__meta">
          {providerLabel(account.provider)} ·{' '}
          {account.login ?? 'Логин не указан'}
        </div>
        <div className="cp-accounts__meta">
          Экземпляр <code>{account.instance_id}</code>
        </div>
        <TechnicalValue
          label="ID в источнике"
          value={account.native_account_id}
        />
      </th>
      <td>
        {unavailable ? (
          'Учётная запись недоступна'
        ) : (
          <>
            {account.assignment && (
              <Label theme="success" size="m">
                Подтверждено
              </Label>
            )}
            <div>{assignmentLabel(account.assignment, people)}</div>
            {account.assignment && (
              <StatusTime value={account.assignment.confirmed_at} />
            )}
          </>
        )}
      </td>
      <td>
        {hasProposedKind && (
          <div className="cp-accounts__suggestion">
            <Label theme="info" size="m">
              Кандидат
            </Label>
            <span>{kindLabel(account.proposed_kind)} по данным источника</span>
          </div>
        )}
        {account.proposals.map(({ person_id, basis }) => {
          const label = `${personName(person_id, people)} · ${contractLabel(basisLabels, basis)}`
          const candidate = {
            kind: 'person',
            personId: person_id,
            basis,
          } as const
          return (
            <div
              className="cp-accounts__suggestion"
              key={`${person_id}/${basis}`}
            >
              <Label theme="info" size="m">
                Кандидат
              </Label>
              {isExactMatch(basis) ? (
                <Checkbox
                  size="l"
                  disabled={disabled || !people.has(person_id)}
                  checked={
                    draft?.decision.kind === 'person' &&
                    draft.decision.personId === person_id &&
                    draft.decision.basis === basis
                  }
                  onUpdate={(checked) => {
                    decide(account, checked ? candidate : null)
                  }}
                >
                  {label}
                </Checkbox>
              ) : (
                <>
                  <span>{label}</span>
                  <Button
                    size="xl"
                    disabled={disabled || !people.has(person_id)}
                    aria-label={`Принять предложение: ${label}, ${name}`}
                    onClick={() => {
                      choose(candidate)
                    }}
                  >
                    Принять предложение
                  </Button>
                </>
              )}
            </div>
          )
        })}
        {!hasCandidates && (
          <span className="cp-accounts__meta">Нет предложений</span>
        )}
      </td>
      <td>
        <div className="cp-account-fields">
          {draft && <div>Выбрано: {decisionLabel(draft, people)}</div>}
          {result && (
            <div>
              <Label
                size="m"
                theme={
                  result.outcome.outcome === 'rejected'
                    ? 'danger'
                    : result.outcome.outcome === 'version_conflict'
                      ? 'warning'
                      : 'success'
                }
              >
                {outcomeLabel(result)}
              </Label>
            </div>
          )}
          <Button
            size="xl"
            disabled={disabled}
            aria-expanded={editing}
            aria-label={`Изменить привязку: ${name}`}
            onClick={() => {
              setEditing(!editing)
            }}
          >
            Изменить привязку
          </Button>
          {editing && (
            <div className="cp-account-fields">
              <PersonPicker
                people={people}
                disabled={disabled}
                onChoose={(personId) => {
                  choose({ kind: 'person', personId })
                }}
              />
              <Button
                size="xl"
                disabled={disabled}
                onClick={() => {
                  choose({ kind: 'create' })
                }}
              >
                Создать человека
              </Button>
              <Button
                size="xl"
                disabled={disabled}
                onClick={() => {
                  choose({ kind: 'bot' })
                }}
              >
                Пометить ботом
              </Button>
              <Button
                size="xl"
                disabled={disabled}
                onClick={() => {
                  choose({ kind: 'system' })
                }}
              >
                Пометить служебной
              </Button>
            </div>
          )}
          {draft && (
            <Button
              size="xl"
              disabled={refreshing}
              onClick={() => {
                row.current?.focus()
                decide(account, null)
              }}
            >
              Убрать решение
            </Button>
          )}
        </div>
      </td>
    </tr>
  )
}
