import type { components, OpenCodes } from '@/shared/api'
import { contractLabel } from '@/shared/config'

type Schema = components['schemas']
export type Account = OpenCodes<Schema['SourceAccount']>
export type Person = OpenCodes<Schema['Person']>
export type Candidate = Account['proposals'][number]
export type Batch = Schema['AccountAssignmentBatch']
type Outcome = Schema['AccountAssignmentBatchResult']['items'][number]

export type Decision =
  | { kind: 'person'; personId: string; basis?: string }
  | { kind: 'create' }
  | { kind: 'bot' | 'system' }

export interface Draft {
  account: Account
  decision: Decision
}

export interface RowResult {
  account: Account
  outcome: Outcome
  baseAssignmentId: string | null
  refresh: 'needed' | 'ready' | 'missing'
}

export const batchLimit = 200

export const basisLabels = {
  login_match: 'совпал логин',
  email_match: 'совпала почта',
  provider_identity: 'подтверждено провайдером входа',
}

export function isExactMatch(basis: string) {
  return basis === 'login_match' || basis === 'email_match'
}

const kindLabels = {
  human: 'Человек',
  bot: 'Бот',
  system: 'Служебная учётная запись',
  unknown: 'Неизвестный вид',
}

export function kindLabel(kind: string) {
  return contractLabel(kindLabels, kind)
}

export function personName(id: string, people: ReadonlyMap<string, Person>) {
  return people.get(id)?.display_name ?? id
}

export function accountLabel(account: Account) {
  const name = account.display_name ?? account.login
  const identity = name
    ? `${name} (${account.native_account_id})`
    : account.native_account_id
  return `${identity} · Экземпляр ${account.instance_id}`
}

export function assignmentLabel(
  assignment: Account['assignment'],
  people: ReadonlyMap<string, Person>,
) {
  if (!assignment) return 'Не сопоставлена'
  const kind = kindLabel(assignment.actor_kind)
  return assignment.person_id
    ? `${personName(assignment.person_id, people)} · ${kind}`
    : kind
}

export function decisionLabel(
  draft: Draft,
  people: ReadonlyMap<string, Person>,
) {
  const { account, decision } = draft
  if (decision.kind === 'person')
    return `${personName(decision.personId, people)} · Человек`
  if (decision.kind === 'create')
    return account.display_name
      ? `Создать человека (${account.display_name})`
      : 'Создать человека'
  return kindLabel(decision.kind)
}

export function assignmentItem({
  account,
  decision,
}: Draft): Batch['items'][number] {
  return {
    source_account_id: account.id,
    person_id: decision.kind === 'person' ? decision.personId : null,
    actor_kind:
      decision.kind === 'bot' || decision.kind === 'system'
        ? decision.kind
        : 'human',
    create_person: decision.kind === 'create',
    base_assignment_id: account.assignment?.id ?? null,
  }
}

export function outcomeLabel({ outcome }: RowResult) {
  switch (outcome.outcome) {
    case 'saved':
      return 'Сохранено'
    case 'unchanged':
      return 'Привязка уже действовала'
    case 'version_conflict':
      return 'Эта привязка уже изменена'
    case 'rejected':
      if (outcome.error_code === 'person_required') return 'Выберите человека'
      if (outcome.error_code === 'person_not_found')
        return 'Выбранный человек недоступен. Выберите другого.'
      if (outcome.error_code === 'not_found')
        return 'Нет доступа к учётной записи'
      return 'Решение отклонено'
  }
}
