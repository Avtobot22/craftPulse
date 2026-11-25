import { matchPath } from 'react-router'
import { routePaths } from '@/shared/config'

const tasksSection = {
  path: routePaths.taskList,
  label: 'Задачи и история',
  action: 'tasks.read',
  children: [{ path: routePaths.projectTimeline, label: 'Таймлайн проекта' }],
}

export const taskAnalysisSections = [
  tasksSection,
  {
    path: routePaths.mergeRequestQueue,
    label: 'MR и CI',
    action: 'mr.read',
    children: [],
  },
]

export function taskAnalysisSection(pathname: string) {
  if (matchPath(routePaths.taskDetail, pathname)) return tasksSection
  return taskAnalysisSections.find(({ path, children }) =>
    [path, ...children.map((child) => child.path)].some((pattern) =>
      matchPath(pattern, pathname),
    ),
  )
}
