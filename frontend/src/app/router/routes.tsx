import type { RouteObject } from 'react-router'
import { PipelineJobPage, PipelinePage } from '@/pages/ci-result'
import { HomePage } from '@/pages/home'
import { MergeRequestSummaryPage } from '@/pages/merge-request-summary'
import { LoginPage } from '@/pages/login'
import { SessionExpiredPage } from '@/pages/session-expired'
import { UnavailablePage } from '@/pages/unavailable'
import { InstallationPage } from '@/pages/installation'
import { WelcomePage } from '@/pages/welcome'
import { OrganizationPage } from '@/pages/organization'
import { UsersPage } from '@/pages/users'
import { WorkflowSettingsPage } from '@/pages/workflow-settings'
import { TargetSettingsPage } from '@/pages/target-settings'
import { MetricPassportPage } from '@/pages/metric-passport'
import { MetricCohortPage } from '@/pages/metric-cohort'
import { TaskListPage } from '@/pages/task-list'
import { TaskDetailPage } from '@/pages/task-detail'
import { ProjectTimelinePage } from '@/pages/project-timeline'
import { MergeRequestQueuePage } from '@/pages/merge-request-queue'
import { routePaths } from '@/shared/config'
import { LoadingState } from '@/shared/ui'
import type { AppRuntime } from '../initialization/runtime'
import { ProtectedRoute } from './protected-route'
import { SessionNavigation } from './session-navigation'
import { adminRoutes } from './admin-routes'
import { technicalRoutes } from './technical-routes'

export function createAppRoutes(runtime: AppRuntime): RouteObject[] {
  return [
    {
      element: <SessionNavigation runtime={runtime} />,
      children: [
        {
          path: routePaths.login,
          element: <LoginPage session={runtime.session} />,
        },
        {
          path: routePaths.login + '/:orgSlug',
          element: <LoginPage session={runtime.session} />,
        },
        {
          path: '/o/:orgSlug',
          element: <LoginPage session={runtime.session} />,
        },
        {
          path: routePaths.sessionExpired,
          element: <SessionExpiredPage session={runtime.session} />,
        },
        {
          element: <ProtectedRoute runtime={runtime} />,
          children: [
            {
              path: routePaths.home,
              element: <HomePage api={runtime.api} session={runtime.session} />,
              children: [
                {
                  path: routePaths.unavailable,
                  element: <UnavailablePage api={runtime.api} />,
                },
                {
                  path: routePaths.installation,
                  element: <InstallationPage api={runtime.api} />,
                },
                { path: routePaths.welcome, element: <WelcomePage /> },
                {
                  path: routePaths.mergeRequestSummary,
                  element: <MergeRequestSummaryPage api={runtime.api} />,
                },
                {
                  path: routePaths.organization,
                  element: <OrganizationPage api={runtime.api} />,
                },
                {
                  path: routePaths.users + '/:userId?',
                  element: (
                    <UsersPage api={runtime.api} session={runtime.session} />
                  ),
                },
                {
                  path: routePaths.pipeline,
                  element: <PipelinePage api={runtime.api} />,
                },
                {
                  path: routePaths.pipelineJob,
                  element: <PipelineJobPage api={runtime.api} />,
                },
                {
                  path: routePaths.workflowSettings,
                  element: <WorkflowSettingsPage api={runtime.api} />,
                },
                {
                  path: routePaths.targetSettings,
                  element: <TargetSettingsPage api={runtime.api} />,
                },
                {
                  path: routePaths.teamReport,
                  HydrateFallback: LoadingState,
                  lazy: async () => {
                    const { TeamReportPage } =
                      await import('@/pages/team-report')
                    return { element: <TeamReportPage api={runtime.api} /> }
                  },
                },
                {
                  path: routePaths.metricPassport,
                  element: <MetricPassportPage api={runtime.api} />,
                },
                {
                  path: routePaths.metricCohort,
                  element: <MetricCohortPage api={runtime.api} />,
                },
                {
                  path: routePaths.savedView,
                  HydrateFallback: LoadingState,
                  lazy: async () => {
                    const { SavedViewPage } =
                      await import('@/pages/team-report')
                    return { element: <SavedViewPage api={runtime.api} /> }
                  },
                },
                {
                  path: routePaths.savedViews,
                  HydrateFallback: LoadingState,
                  lazy: async () => {
                    const { SavedViewsPage } =
                      await import('@/pages/team-report')
                    return { element: <SavedViewsPage api={runtime.api} /> }
                  },
                },
                {
                  path: routePaths.taskList,
                  element: <TaskListPage api={runtime.api} />,
                },
                {
                  path: routePaths.taskDetail,
                  element: <TaskDetailPage api={runtime.api} />,
                },
                {
                  path: routePaths.projectTimeline,
                  element: <ProjectTimelinePage api={runtime.api} />,
                },
                {
                  path: routePaths.mergeRequestQueue,
                  element: <MergeRequestQueuePage api={runtime.api} />,
                },
                ...adminRoutes,
                ...technicalRoutes(runtime.api),
                { path: '*', element: <UnavailablePage api={runtime.api} /> },
              ],
            },
          ],
        },
      ],
    },
  ]
}
