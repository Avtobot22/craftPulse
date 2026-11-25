import { Button, Container, Grid, Text } from '@gravity-ui/uikit'
import { useEffect, useRef, useState } from 'react'
import type { CSSProperties } from 'react'
import {
  Link,
  matchPath,
  Navigate,
  useLocation,
  useNavigate,
  useOutlet,
  useOutletContext,
} from 'react-router'
import type { ApiClient } from '@/shared/api'
import type { CurrentUser, SessionController } from '@/shared/auth'
import { contentGridProps, layoutSizes, routePaths } from '@/shared/config'
import { ErrorState } from '@/shared/ui'
import {
  changeScope,
  projectParam,
  rememberScope,
  teamParam,
} from '../model/scope'
import { useScope } from '../model/use-scope'
import './home-page.css'

const report = {
  path: routePaths.teamReport,
  action: 'report.read',
  label: 'Отчёт команды',
}

const reportPaths = [
  report.path,
  routePaths.metricPassport,
  routePaths.metricCohort,
  routePaths.savedView,
]

const settings = [
  {
    path: routePaths.installation,
    action: 'installation.read',
    label: 'Диагностика установки',
  },
  {
    path: routePaths.organization,
    action: 'org.manage',
    label: 'Структура организации',
  },
  {
    path: routePaths.users,
    action: 'users.manage',
    label: 'Пользователи и роли',
  },
  {
    path: routePaths.workflowSettings,
    action: 'process.manage',
    label: 'Настройки процесса',
  },
  {
    path: routePaths.targetSettings,
    action: 'process.manage',
    label: 'Целевые сроки',
  },
]

const views = [
  { value: 'overview', label: 'Обзор', action: 'report.read' },
  { value: 'tasks', label: 'Задачи и история', action: 'tasks.read' },
  { value: 'delivery', label: 'MR и CI', action: 'mr.read' },
  { value: 'security', label: 'Код и безопасность', action: 'findings.read' },
  { value: 'docs', label: 'Документация', action: 'docs.read' },
]

export function HomePage({
  api,
  session,
}: {
  api: ApiClient
  session: SessionController
}) {
  const current = useOutletContext<CurrentUser>()
  const location = useLocation()
  const navigate = useNavigate()
  const outlet = useOutlet()
  const content = useRef<HTMLElement>(null)
  const navButton = useRef<HTMLButtonElement>(null)
  const [menuOpen, setMenuOpen] = useState(false)
  const [logout, setLogout] = useState<'idle' | 'pending' | 'failed'>('idle')
  const hasAreas = Object.values(current.areas).some(
    (areas) => areas.length > 0,
  )
  const storageKey = `craftpulse.scope.${current.organization.id}.${current.user.id}`
  const params = new URLSearchParams(location.search)
  const isHome = location.pathname === routePaths.home
  const state: unknown = location.state
  const skipScopeRestore =
    state && typeof state === 'object' && 'skipScopeRestore' in state
  const restore =
    isHome &&
    !location.search &&
    !location.hash &&
    hasAreas &&
    !skipScopeRestore
  const scope = useScope(api, hasAreas, storageKey, params, restore)
  const active = settings.find(
    ({ path }) =>
      matchPath(path, location.pathname) ??
      (path === routePaths.users &&
        matchPath(path + '/:userId', location.pathname)),
  )
  const allowed = settings.filter(({ action }) =>
    current.actions.includes(action),
  )
  const visibleSettings = allowed.filter(
    ({ action }) => action !== 'process.manage' || scope.team,
  )
  const currentSetting = active && allowed.includes(active) ? active : undefined
  const isReport = reportPaths.some((path) =>
    matchPath(path, location.pathname),
  )
  const availableViews = views.filter(({ action }) =>
    current.actions.includes(action),
  )
  const missing =
    Boolean(active && !currentSetting) ||
    ((isReport || currentSetting?.action === 'process.manage') &&
      scope.invalid) ||
    (isReport && !current.actions.includes(report.action))
  const noArea =
    !hasAreas &&
    !currentSetting &&
    location.pathname !== routePaths.welcome &&
    location.pathname !== routePaths.unavailable
  const areaMessage =
    state &&
    typeof state === 'object' &&
    'scopeMessage' in state &&
    typeof state.scopeMessage === 'string'
      ? state.scopeMessage
      : null
  const hasRememberedTeam = scope.remembered?.has(teamParam) ?? false
  const restoring = Boolean(hasRememberedTeam && scope.ready && scope.team)
  // An administrator without structure can configure it before any area exists.
  const setup =
    isHome &&
    !location.search &&
    !location.hash &&
    !hasAreas &&
    current.actions.includes('org.manage')

  useEffect(() => {
    const main = content.current
    const heading = main?.querySelector('h1')
    if (heading) heading.tabIndex = -1
    const target = heading ?? main
    target?.focus()
  }, [location.pathname, missing, noArea, restoring])

  function updateArea(team: string, project?: string) {
    const { params: next, reset } = changeScope(params, team, project)
    const nextPath =
      active?.path ?? (isReport ? routePaths.teamReport : routePaths.home)
    if (!team) rememberScope(storageKey, '', '')
    void navigate(
      { pathname: nextPath, search: next.toString() },
      {
        state: {
          scopeMessage:
            reset || location.hash || nextPath !== location.pathname
              ? 'Несовместимые параметры сброшены при смене области.'
              : null,
        },
      },
    )
  }

  async function signOut() {
    setLogout('pending')
    try {
      await session.logout()
    } catch {
      setLogout('failed')
    }
  }

  function goTo(pathname: string) {
    const { params: next } = changeScope(
      params,
      params.get(teamParam) ?? '',
      params.get(projectParam) ?? '',
    )
    return { pathname, search: next.toString() }
  }

  if (setup) return <Navigate to={routePaths.organization} replace />

  if (restoring && scope.team) {
    const { params: previous } = changeScope(
      new URLSearchParams(),
      scope.team.id,
      scope.project?.id,
    )
    return (
      <Navigate
        to={{ pathname: routePaths.home, search: previous.toString() }}
        state={
          scope.invalid
            ? { scopeMessage: 'Последний проект недоступен.' }
            : null
        }
        replace
      />
    )
  }

  const teamError = scope.teams.isError && hasAreas
  const projectError = scope.projects.isError && !!scope.team
  const overview = hasAreas && current.actions.includes(report.action)
  const hasNavigation = overview || visibleSettings.length > 0
  return (
    <div
      className={hasNavigation ? 'cp-shell' : 'cp-shell cp-shell--without-nav'}
      style={
        { '--cp-navigation-width': layoutSizes.navigation } as CSSProperties
      }
    >
      <a
        className="cp-shell__skip"
        href="#main-content"
        onClick={(event) => {
          event.preventDefault()
          const heading = content.current?.querySelector('h1')
          const target = heading ?? content.current
          target?.focus()
        }}
      >
        К содержимому
      </a>
      <header className="cp-shell__header">
        <Text variant="subheader-2">{current.organization.name}</Text>
        {hasAreas && (
          <div
            className="cp-shell__controls"
            role="group"
            aria-label="Выбор области"
          >
            <label>
              Команда
              <select
                value={scope.team?.id ?? ''}
                disabled={!scope.teams.isSuccess}
                onChange={(event) => {
                  updateArea(event.target.value)
                }}
              >
                <option value="">
                  {scope.teams.isPending
                    ? 'Загрузка команд…'
                    : 'Выберите команду'}
                </option>
                {!teamError &&
                  scope.teams.data?.map(({ id, name }) => (
                    <option key={id} value={id}>
                      {name}
                    </option>
                  ))}
              </select>
            </label>
            <label>
              Проект
              <select
                value={scope.project?.id ?? ''}
                disabled={!scope.team || !scope.projects.isSuccess}
                onChange={(event) => {
                  updateArea(scope.team?.id ?? '', event.target.value)
                }}
              >
                <option value="">
                  {scope.team && scope.projects.isPending
                    ? 'Загрузка проектов…'
                    : 'Выберите проект'}
                </option>
                {scope.team &&
                  !projectError &&
                  scope.projects.data?.map(({ id, name }) => (
                    <option key={id} value={id}>
                      {name}
                    </option>
                  ))}
              </select>
            </label>
          </div>
        )}
        {hasAreas && availableViews.length > 0 && (
          <label>
            Вид панели
            <select
              disabled={
                hasRememberedTeam &&
                (scope.teams.isPending ||
                  Boolean(scope.team && scope.projects.isPending))
              }
              value={
                availableViews.find(({ value }) => value === params.get('view'))
                  ?.value ?? ''
              }
              onChange={(event) => {
                const value = event.target.value
                if (value) params.set('view', value)
                else params.delete('view')
                void navigate(
                  {
                    pathname: location.pathname,
                    search: params.toString(),
                    hash: location.hash,
                  },
                  {
                    state: {
                      scopeMessage: areaMessage,
                      skipScopeRestore: true,
                    },
                  },
                )
              }}
            >
              <option value="">По умолчанию</option>
              {availableViews.map(({ value, label }) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </select>
          </label>
        )}
        <div className="cp-shell__profile" role="group" aria-label="Профиль">
          <span>{current.user.display_name ?? 'Профиль'}</span>
          <Button loading={logout === 'pending'} onClick={() => void signOut()}>
            Выход
          </Button>
        </div>
      </header>
      {hasNavigation && (
        <div
          className="cp-shell__navigation"
          onKeyDown={(event) => {
            if (event.key === 'Escape' && menuOpen) {
              setMenuOpen(false)
              navButton.current?.focus()
            }
          }}
        >
          <button
            className="cp-shell__menu"
            ref={navButton}
            type="button"
            aria-expanded={menuOpen}
            aria-controls="main-navigation"
            onClick={() => {
              setMenuOpen(!menuOpen)
            }}
          >
            Разделы
          </button>
          <nav
            id="main-navigation"
            aria-label="Основная навигация"
            data-open={menuOpen}
          >
            {overview && (
              <Link
                aria-current={isHome ? 'page' : undefined}
                to={goTo(routePaths.home)}
                onClick={() => {
                  setMenuOpen(false)
                  if (isHome && menuOpen) navButton.current?.focus()
                }}
              >
                Обзор
              </Link>
            )}
            {overview && scope.team && (
              <Link
                aria-current={
                  location.pathname === report.path
                    ? 'page'
                    : isReport
                      ? 'location'
                      : undefined
                }
                to={goTo(report.path)}
                onClick={() => {
                  setMenuOpen(false)
                  if (location.pathname === report.path && menuOpen)
                    navButton.current?.focus()
                }}
              >
                {report.label}
              </Link>
            )}
            {visibleSettings.length > 0 && (
              <div role="group" aria-labelledby="settings-heading">
                <Text id="settings-heading" as="h2" variant="subheader-1">
                  Настройки
                </Text>
                {visibleSettings.map(({ path, label }) => (
                  <Link
                    key={path}
                    aria-current={active?.path === path ? 'page' : undefined}
                    to={goTo(path)}
                    onClick={() => {
                      setMenuOpen(false)
                      if (location.pathname === path && menuOpen)
                        navButton.current?.focus()
                    }}
                  >
                    {label}
                  </Link>
                ))}
              </div>
            )}
          </nav>
        </div>
      )}
      <main
        id="main-content"
        ref={content}
        tabIndex={-1}
        className="cp-shell__main"
      >
        <Container
          maxWidth={layoutSizes.content}
          paddingBlock={{ xs: 'spacing-4', m: 'spacing-6', xl: 'spacing-8' }}
        >
          <Grid {...contentGridProps}>
            <div className="cp-shell__content">
              {logout === 'failed' && (
                <p role="alert">
                  Не удалось завершить выход. Повторите попытку.
                </p>
              )}
              {areaMessage && <p role="status">{areaMessage}</p>}
              {scope.invalid && !restore && !noArea && (
                <p role="status">
                  Объект или область недоступны. Выберите доступную область.
                </p>
              )}
              {(teamError || projectError) && (
                <ErrorState
                  reason={
                    teamError
                      ? 'Не удалось загрузить команды'
                      : 'Не удалось загрузить проекты'
                  }
                  retrying={scope.teams.isFetching || scope.projects.isFetching}
                  onRetry={() => {
                    const query = teamError ? scope.teams : scope.projects
                    void query.refetch()
                  }}
                />
              )}
              {(currentSetting ?? (isHome && scope.team)) && (
                <nav aria-label="Хлебные крошки" className="cp-shell__crumbs">
                  <ol>
                    {scope.team && <li>{scope.team.name}</li>}
                    {scope.project && <li>{scope.project.name}</li>}
                    {currentSetting && <li>Настройки</li>}
                    <li aria-current="page">
                      {currentSetting?.label ?? 'Обзор'}
                    </li>
                  </ol>
                </nav>
              )}
              {missing ? (
                <Text as="h1" variant="header-1">
                  Объект или область недоступны
                </Text>
              ) : noArea ? (
                <section>
                  <Text as="h1" variant="header-1">
                    Нет доступной области
                  </Text>
                  <p>Обратитесь к администратору.</p>
                </section>
              ) : (
                (outlet ?? (
                  <section>
                    <Text as="h1" variant="header-1">
                      Обзор
                    </Text>
                    {scope.teams.data?.length === 0 ? (
                      <p>Нет доступных команд. Обратитесь к администратору.</p>
                    ) : !scope.team && !teamError ? (
                      <p>Выберите команду и проект.</p>
                    ) : null}
                  </section>
                ))
              )}
            </div>
          </Grid>
        </Container>
      </main>
    </div>
  )
}
