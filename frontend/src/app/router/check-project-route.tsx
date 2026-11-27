import { Text } from '@gravity-ui/uikit'
import type { ReactNode } from 'react'
import { generatePath, Link, useParams, useOutletContext } from 'react-router'
import type { CurrentUser } from '@/shared/auth'
import type { TeamReportScope } from '@/shared/config'

type ProjectScope = Pick<CurrentUser, 'actions' | 'areas'> &
  Pick<TeamReportScope, 'projects'>

export function CheckProjectRoute({
  title,
  path,
  children,
}: {
  title: string
  path: string
  children: (projectId: string, actions: readonly string[]) => ReactNode
}) {
  const { projectId } = useParams()
  const scope = useOutletContext<ProjectScope>()
  const projects = new Map(
    [...scope.areas.projects, ...scope.projects].map((project) => [
      project.id,
      project,
    ]),
  )

  return (
    <section>
      <Text as="h1" variant="header-1">
        {title}
      </Text>
      {!projectId && (
        <>
          {projects.size > 0 ? (
            <nav aria-label="Выбор проекта">
              <p>Выберите проект</p>
              <ul>
                {Array.from(projects.values(), (project) => (
                  <li key={project.id}>
                    <Link to={generatePath(path, { projectId: project.id })}>
                      {project.name}
                    </Link>
                  </li>
                ))}
              </ul>
            </nav>
          ) : (
            <p>Выберите команду с доступными проектами.</p>
          )}
        </>
      )}
      {projectId && (
        <>
          <p>
            Проект {projects.get(projectId)?.name ?? projectId} ·{' '}
            <Link to={generatePath(path)}>Выбрать проект</Link>
          </p>
          {children(projectId, scope.actions)}
        </>
      )}
    </section>
  )
}
