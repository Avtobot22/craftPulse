import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { HomePage } from '@/pages/home'
import './styles/global.css'

const root = document.getElementById('root')
if (!root) throw new Error('Root element is missing')

createRoot(root).render(
  <StrictMode>
    <HomePage />
  </StrictMode>,
)
