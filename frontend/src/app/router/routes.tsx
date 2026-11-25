import type { RouteObject } from 'react-router'
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
import { TeamReportPage } from '@/pages/team-report'
import { MetricPassportPage } from '@/pages/metric-passport'
import { MetricCohortPage } from '@/pages/metric-cohort'
import { SavedViewPage } from '@/pages/saved-view'
import { TaskListPage } from '@/pages/task-list'
import { TaskDetailPage } from '@/pages/task-detail'
import { ProjectTimelinePage } from '@/pages/project-timeline'
import { MergeRequestQueuePage } from '@/pages/merge-request-queue'
import { routePaths } from '@/shared/config'
import type { AppRuntime } from '../initialization/runtime'
import { ProtectedRoute } from './protected-route'
import { SessionNavigation } from './session-navigation'
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
                { path: routePaths.unavailable, element: <UnavailablePage /> },
                {
                  path: routePaths.installation,
                  element: <InstallationPage />,
                },
                { path: routePaths.welcome, element: <WelcomePage /> },
                {
                  path: routePaths.mergeRequestSummary,
                  element: <MergeRequestSummaryPage api={runtime.api} />,
                },
                {
                  path: routePaths.organization,
                  element: <OrganizationPage />,
                },
                { path: routePaths.users, element: <UsersPage /> },
                { path: routePaths.users + '/:userId', element: <UsersPage /> },
                {
                  path: routePaths.workflowSettings,
                  element: <WorkflowSettingsPage />,
                },
                {
                  path: routePaths.targetSettings,
                  element: <TargetSettingsPage />,
                },
                {
                  path: routePaths.teamReport,
                  element: <TeamReportPage />,
                },
                {
                  path: routePaths.metricPassport,
                  element: <MetricPassportPage />,
                },
                {
                  path: routePaths.metricCohort,
                  element: <MetricCohortPage />,
                },
                {
                  path: routePaths.savedView,
                  element: <SavedViewPage />,
                },
                { path: routePaths.taskList, element: <TaskListPage /> },
                { path: routePaths.taskDetail, element: <TaskDetailPage /> },
                {
                  path: routePaths.projectTimeline,
                  element: <ProjectTimelinePage />,
                },
                {
                  path: routePaths.mergeRequestQueue,
                  element: <MergeRequestQueuePage />,
                },
                ...technicalRoutes,
                { path: '*', element: <UnavailablePage /> },
              ],
            },
          ],
        },
      ],
    },
  ]
}
