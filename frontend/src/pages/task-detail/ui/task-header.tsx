import { Text } from '@gravity-ui/uikit'
import { SourceLink } from '@/entities/activity'
import { contractLabel, contractLabels } from '@/shared/config'
import type { TaskDetail } from '../model/task-evidence'

export function TaskHeader({
  task,
  teamName,
}: {
  task: TaskDetail | undefined
  teamName: string | undefined
}) {
  const heading = [task?.key, task?.title].filter(Boolean).join(' · ')
  return (
    <header>
      <Text as="h1" variant="header-2">
        <SourceLink url={task?.source_url ?? null}>
          {heading || 'Карточка задачи'}
        </SourceLink>
      </Text>
      {task && (
        <dl className="cp-task-detail__meta">
          <div>
            <dt>Статус</dt>
            <dd>
              {contractLabel(contractLabels.taskStatus, task.status.meaning)}
              {task.status.name && ` · ${task.status.name}`}
            </dd>
          </div>
          <div>
            <dt>Тип</dt>
            <dd>{task.task_type ?? 'Неизвестен'}</dd>
          </div>
          <div>
            <dt>Размер</dt>
            <dd>{task.size_label ?? 'Без размера'}</dd>
          </div>
          <div>
            <dt>Команда</dt>
            <dd>{teamName ?? 'Команда не определена'}</dd>
          </div>
        </dl>
      )}
    </header>
  )
}
