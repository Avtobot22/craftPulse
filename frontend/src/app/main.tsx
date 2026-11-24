import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { createBrowserRouter } from 'react-router'
import { RouterProvider } from 'react-router/dom'
import '@fontsource/inter/cyrillic-400.css'
import '@fontsource/inter/cyrillic-600.css'
import '@fontsource/inter/latin-400.css'
import '@fontsource/inter/latin-600.css'
import '@gravity-ui/uikit/styles/styles.css'
import './styles/theme.css'
import './styles/controls.css'
import './styles/global.css'
import { AppProviders } from './initialization/app-providers'
import { createAppRuntime } from './initialization/runtime'
import { createAppRoutes } from './router/routes'

const root = document.getElementById('root')
if (!root) throw new Error('Root element is missing')
const runtime = createAppRuntime()
const router = createBrowserRouter(createAppRoutes(runtime))
createRoot(root).render(
  <StrictMode>
    <AppProviders runtime={runtime}>
      <RouterProvider router={router} />
    </AppProviders>
  </StrictMode>,
)
