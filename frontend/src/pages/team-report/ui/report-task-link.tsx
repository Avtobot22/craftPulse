import type { ReactNode } from 'react'
import { useSearchParams } from 'react-router'
import type { TeamReport } from '@/shared/api'
import { TaskDetailLink } from '@/shared/ui'

export function ReportTaskLink({
  report,
  taskId,
  point,
  pointParam,
  focusId,
  children,
}: {
  report: TeamReport
  taskId: string
  point: string
  pointParam: 'work_age_point' | 'cross_point' | 'technical_point'
  focusId: string
  children: ReactNode
}) {
  const [search] = useSearchParams()
  const projectId = search.get('project_id')
  const returnSearch = new URLSearchParams(search)
  returnSearch.set('revision_id', report.revision.id)
  returnSearch.set(pointParam, point)

  return (
    <TaskDetailLink
      taskId={taskId}
      focusId={focusId}
      params={{
        team_id: report.team_id,
        ...(projectId ? { project_id: projectId } : {}),
        ...report.period,
        revision_id: report.revision.id,
        as_of: report.revision.as_of,
      }}
      preserveHistoryFocus
      returnSearch={'?' + returnSearch.toString()}
    >
      {children}
    </TaskDetailLink>
  )
}
