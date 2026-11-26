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
import type { ApiClient, Team } from '@/shared/api'
import type { CurrentUser, SessionController } from '@/shared/auth'
import {
  contentGridProps,
  layoutSizes,
  readTaskOrigin,
  readTaskReturnFocus,
  routePaths,
} from '@/shared/config'
import type { ReportPointFocus, TeamReportScope } from '@/shared/config'
import { ErrorState, LoadingState } from '@/shared/ui'
import { adminDetailRoutes, adminNavigation } from '../model/admin-navigation'
import {
  changeScope,
  projectParam,
  rememberScope,
  teamParam,
} from '../model/scope'
import {
  taskAnalysisSection,
  taskAnalysisSections,
} from '../model/task-analysis-navigation'
import {
  technicalNavigation,
  technicalRouteAction,
} from '../model/technical-navigation'
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
  routePaths.savedViews,
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
  ...adminNavigation,
]

const settingRoutes = [...settings, ...adminDetailRoutes]

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
  const [reportFocus, setReportFocus] = useState<ReportPointFocus>()
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
  const isTaskDetail =
    matchPath(routePaths.taskDetail, location.pathname) !== null
  const analysisSection = taskAnalysisSection(location.pathname)
  const technicalAction = technicalRouteAction(location.pathname)
  const isFindingsList = location.pathname === routePaths.findings
  const technicalLinks = hasAreas
    ? technicalNavigation.filter(({ action }) =>
        current.actions.includes(action),
      )
    : []
  const scope = useScope(
    api,
    hasAreas,
    storageKey,
    params,
    restore,
    analysisSection || isFindingsList ? current.areas.projects : undefined,
  )
  const active = settingRoutes.find(
    ({ path }) =>
      matchPath(path, location.pathname) ??
      (path === routePaths.users &&
        matchPath(path + '/:userId', location.pathname)),
  )
  const selectedTeamId = scope.team?.id
  const ownsProcess =
    selectedTeamId !== undefined &&
    current.roles.some(
      ({ role, team_id }) =>
        role === 'process_owner' && team_id === selectedTeamId,
    )
  const canOpenSetting = ({ action }: { action: string }) =>
    current.actions.includes(action) &&
    (action !== 'process.manage' || ownsProcess)
  const allowed = settings.filter(canOpenSetting)
  const outlet = useOutlet({
    ...current,
    teams: scope.teams.data,
    ...(scope.team ? { team: scope.team } : {}),
    ...(scope.project ? { project: scope.project } : {}),
    projects: scope.projectChoices ?? [],
    canManageProcess: allowed.some(({ action }) => action === 'process.manage'),
    focus: reportFocus?.locationKey === location.key ? reportFocus : undefined,
    rememberPoint: setReportFocus,
  } satisfies CurrentUser & TeamReportScope & { teams: Team[] | undefined })
  const currentSetting = active && canOpenSetting(active) ? active : undefined
  const isProcessSetting = active?.action === 'process.manage'
  const isReport = reportPaths.some((path) =>
    matchPath(path, location.pathname),
  )
  const canReadReport = current.actions.includes(report.action)
  const canManageProcess =
    isProcessSetting && current.actions.includes('process.manage')
  const canReadAnalysis =
    analysisSection !== undefined &&
    current.actions.includes(analysisSection.action)
  const hasProjectParam = Boolean(params.get(projectParam))
  const hasTeamParam = Boolean(params.get(teamParam))
  const checkingTeam =
    (isReport
      ? canReadReport
      : canManageProcess || (canReadAnalysis && hasTeamParam)) &&
    hasAreas &&
    (hasTeamParam || hasProjectParam) &&
    !scope.teams.isSuccess
  const checkingProject =
    (isReport
      ? canReadReport
      : canReadAnalysis || currentSetting?.action === 'process.manage') &&
    hasProjectParam &&
    Boolean(scope.team) &&
    !scope.projects.isSuccess
  const checkingScope = checkingTeam || checkingProject
  const availableViews = views.filter(({ action }) =>
    current.actions.includes(action),
  )
  const analysisNavigation = hasAreas
    ? taskAnalysisSections.filter(({ action }) =>
        current.actions.includes(action),
      )
    : []
  const missing =
    Boolean(active && !currentSetting) ||
    ((isReport || currentSetting?.action === 'process.manage') &&
      scope.invalid) ||
    (isReport && !canReadReport) ||
    (technicalAction !== undefined &&
      !current.actions.includes(technicalAction)) ||
    Boolean(analysisSection && (!canReadAnalysis || scope.invalid))
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

  const focusId = readTaskReturnFocus(state)
  useEffect(() => {
    const main = content.current
    const heading = main?.querySelector('h1')
    if (heading) heading.tabIndex = -1
    const opener = focusId ? document.getElementById(focusId) : null
    const target = opener && main?.contains(opener) ? opener : (heading ?? main)
    target?.focus()
  }, [location.pathname, focusId, missing, noArea, restoring, checkingScope])

  function updateArea(team: string, project?: string) {
    const nextPath = analysisSection
      ? isTaskDetail
        ? routePaths.taskList
        : location.pathname
      : isFindingsList
        ? routePaths.findings
        : (active?.path ?? (isReport ? routePaths.teamReport : routePaths.home))
    const { params: next, reset } = changeScope(params, team, project, {
      projectOnly: analysisSection !== undefined || isFindingsList,
      keepDate: nextPath === routePaths.projectTimeline,
    })
    if (!team && !project) rememberScope(storageKey, '', '')
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
    if (pathname === location.pathname && analysisSection) {
      return { pathname, search: location.search, hash: location.hash }
    }
    const { params: next } = changeScope(
      params,
      params.get(teamParam) ?? '',
      params.get(projectParam) ?? '',
      {
        projectOnly:
          pathname === routePaths.findings ||
          Boolean(taskAnalysisSection(pathname)),
        keepDate: pathname === routePaths.projectTimeline,
      },
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
  const overview = hasAreas && canReadReport
  const hasNavigation =
    overview ||
    allowed.length > 0 ||
    analysisNavigation.length > 0 ||
    technicalLinks.length > 0
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
          if (heading) heading.tabIndex = -1
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
                disabled={!scope.projectChoices}
                onChange={(event) => {
                  updateArea(scope.team?.id ?? '', event.target.value)
                }}
              >
                <option value="">
                  {scope.team && scope.projects.isPending
                    ? 'Загрузка проектов…'
                    : 'Выберите проект'}
                </option>
                {!projectError &&
                  scope.projectChoices?.map(({ id, name }) => (
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
            {analysisNavigation.map(({ path, label, children }) => (
              <div key={path}>
                {[{ path, label }, ...children].map((item) => (
                  <Link
                    key={item.path}
                    aria-current={
                      location.pathname === item.path
                        ? 'page'
                        : item.path === path && analysisSection?.path === path
                          ? 'location'
                          : undefined
                    }
                    to={goTo(item.path)}
                    onClick={() => {
                      setMenuOpen(false)
                      if (location.pathname === item.path && menuOpen)
                        navButton.current?.focus()
                    }}
                  >
                    {item.label}
                  </Link>
                ))}
              </div>
            ))}
            {technicalLinks.map(({ path, pattern, label }) => {
              const isCurrent = matchPath(pattern, location.pathname) !== null
              return (
                <Link
                  key={path}
                  aria-current={isCurrent ? 'page' : undefined}
                  to={isCurrent ? location : goTo(path)}
                  onClick={() => {
                    setMenuOpen(false)
                    if (isCurrent && menuOpen) navButton.current?.focus()
                  }}
                >
                  {label}
                </Link>
              )
            })}
            {allowed.length > 0 && (
              <div role="group" aria-labelledby="settings-heading">
                <Text id="settings-heading" as="h2" variant="subheader-1">
                  Настройки
                </Text>
                {allowed.map(({ path, label }) => (
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
              {isTaskDetail && (missing || noArea) ? (
                <Navigate
                  to={routePaths.unavailable}
                  replace
                  state={{ taskOrigin: readTaskOrigin(state) }}
                />
              ) : checkingScope ? (
                teamError || projectError ? null : (
                  <LoadingState />
                )
              ) : missing ? (
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
