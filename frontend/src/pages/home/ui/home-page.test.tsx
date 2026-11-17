import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { describe, expect, it } from 'vitest'
import { HomePage } from './home-page'

describe('HomePage', () => {
  it('renders the application placeholder', () => {
    render(
      <MemoryRouter>
        <HomePage />
      </MemoryRouter>,
    )
    expect(
      screen.getByRole('heading', { name: 'craftPulse', level: 1 }),
    ).toBeInTheDocument()
  })
})
