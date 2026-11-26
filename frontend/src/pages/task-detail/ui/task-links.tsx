import { useState } from 'react'
import { useLocation, useNavigate } from 'react-router'
import { SourceLink } from '@/entities/activity'
import { LinkDecisionActions, LinkDecisionDialog } from '@/features/decide-link'
import type { LinkDecisionSelection } from '@/features/decide-link'
import type { ApiClient } from '@/shared/api'
import {
  contractLabel,
  contractLabels,
  readTaskOrigin,
  routePaths,
} from '@/shared/config'
import { basisLabel } from '../model/task-evidence'
import type { TaskDetail } from '../model/task-evidence'
import type { TaskLink } from '../model/task-links'

function LinkReference({ link }: { link: TaskLink }) {
  return (
    <SourceLink url={link.to.source_url}>
      {basisLabel(link.to) || 'Запрос на слияние'}
    </SourceLink>
  )
}

function candidateBasis(link: TaskLink, key: string | null) {
  if (link.method === 'branch_key')
    return key
      ? `Ключ ${key} найден в имени ветки MR.`
      : 'Ключ задачи найден в имени ветки MR.'
  if (link.method === 'title_key')
    return key
      ? `Ключ ${key} найден в названии MR.`
      : 'Ключ задачи найден в названии MR.'
  return null
}

export function TaskLinks({
  api,
  task,
  confirmed,
  candidates,
  hasMerged,
  canDecide,
  refreshTask,
}: {
  api: ApiClient
  task: TaskDetail
  confirmed: TaskLink[]
  candidates: TaskLink[]
  hasMerged: boolean
  canDecide: boolean
  refreshTask: () => Promise<void>
}) {
  const [selection, setSelection] = useState<LinkDecisionSelection>()
  const [forbidden, setForbidden] = useState(false)
  const navigate = useNavigate()
  const location = useLocation()
  let coverage = 'Связь с MR не найдена'
  if (task.redactions.includes('links')) coverage = 'Часть связей недоступна'
  if (candidates.length > 0 && !task.redactions.includes('links'))
    coverage = 'Есть только кандидаты связи с MR'
  if (confirmed.length > 0) coverage = 'Есть подтверждённый MR'
  if (hasMerged) coverage = 'Есть подтверждённый слитый MR'

  return (
    <section aria-labelledby="task-links">
      <h2 id="task-links">Связи с MR</h2>
      <p>{coverage}</p>
      {confirmed.length > 0 && (
        <>
          <h3>Подтверждённые связи</h3>
          <ul
            className="cp-task-detail__cards"
            aria-label="Подтверждённые связи"
          >
            {confirmed.map((link) => (
              <li key={link.to.id}>
                <LinkReference link={link} />
                {' · '}
                {contractLabel(contractLabels.linkMethod, link.method)}
              </li>
            ))}
          </ul>
        </>
      )}
      {candidates.length > 0 && (
        <>
          <h3>Кандидаты связи</h3>
          <ul className="cp-task-detail__cards" aria-label="Кандидаты связи">
            {candidates.map((link) => {
              const label = basisLabel(link.to) || 'Запрос на слияние'
              const basis = candidateBasis(link, task.key)
              return (
                <li key={link.id}>
                  <LinkReference link={link} />
                  {' · '}
                  {contractLabel(contractLabels.linkMethod, link.method)}
                  {basis && <p>{basis}</p>}
                  {canDecide && !forbidden && (
                    <LinkDecisionActions
                      linkId={link.id}
                      targetLabel={label}
                      onSelect={setSelection}
                    />
                  )}
                </li>
              )
            })}
          </ul>
        </>
      )}
      {selection && (
        <LinkDecisionDialog
          api={api}
          selection={selection}
          taskLabel={
            [task.key, task.title].filter(Boolean).join(' · ') ||
            'Карточка задачи'
          }
          refreshTask={refreshTask}
          onForbidden={() => {
            setForbidden(true)
          }}
          onNotFound={() =>
            void navigate(routePaths.unavailable, {
              replace: true,
              state: { taskOrigin: readTaskOrigin(location.state) },
            })
          }
          onClose={() => {
            setSelection(undefined)
          }}
        />
      )}
    </section>
  )
}
