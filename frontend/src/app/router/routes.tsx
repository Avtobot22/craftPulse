import type { RouteObject } from 'react-router'
import { HomePage } from '@/pages/home'
import { LoginPage } from '@/pages/login'
import { SessionExpiredPage } from '@/pages/session-expired'
import { UnavailablePage } from '@/pages/unavailable'
import { InstallationPage } from '@/pages/installation'
import { WelcomePage } from '@/pages/welcome'
import { OrganizationPage } from '@/pages/organization'
import { UsersPage } from '@/pages/users'
import { routePaths } from '@/shared/config'
import type { AppRuntime } from '../initialization/runtime'
import { ProtectedRoute } from './protected-route'
import { SessionNavigation } from './session-navigation'

export function createAppRoutes(runtime: AppRuntime): RouteObject[] {
  return [
    {
      element: <SessionNavigation runtime={runtime} />,
      children: [
        { path: routePaths.login, element: <LoginPage /> },
        { path: routePaths.login + '/:orgSlug', element: <LoginPage /> },
        { path: routePaths.sessionExpired, element: <SessionExpiredPage /> },
        {
          element: <ProtectedRoute runtime={runtime} />,
          children: [
            {
              path: routePaths.home,
              element: <HomePage />,
              children: [
                { path: routePaths.unavailable, element: <UnavailablePage /> },
                {
                  path: routePaths.installation,
                  element: <InstallationPage />,
                },
                { path: routePaths.welcome, element: <WelcomePage /> },
                {
                  path: routePaths.organization,
                  element: <OrganizationPage />,
                },
                { path: routePaths.users, element: <UsersPage /> },
                { path: routePaths.users + '/:userId', element: <UsersPage /> },
                { path: '*', element: <UnavailablePage /> },
              ],
            },
          ],
        },
      ],
    },
  ]
}
