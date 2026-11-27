import type { TaskDetail } from './task-evidence'

export type TaskLink = TaskDetail['links'][number]

export function taskLinks(task: TaskDetail) {
  const links = task.links.filter(
    ({ link_type, from_object_id, to }) =>
      link_type === 'task_mr' &&
      from_object_id === task.id &&
      to.kind === 'merge_request',
  )
  const confirmed = new Map<string, TaskLink>()
  for (const link of links) {
    if (link.status === 'confirmed' && !confirmed.has(link.to.id))
      confirmed.set(link.to.id, link)
  }
  const candidates = links.filter(
    (link) => link.status === 'candidate' && !confirmed.has(link.to.id),
  )
  const runs = task.links.filter(
    ({ link_type, status, from_object_id, to }) =>
      link_type === 'mr_run' &&
      status === 'confirmed' &&
      to.kind === 'pipeline' &&
      confirmed.has(from_object_id),
  )
  return { confirmed: [...confirmed.values()], candidates, runs }
}
