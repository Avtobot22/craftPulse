import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { HomePage } from './home-page'

describe('HomePage', () => {
  it('renders the starter and the API error title', () => {
    render(
      <HomePage
        problem={{
          type: 'urn:craftpulse:problem:not_found',
          title: 'Нет данных',
          status: 404,
          code: 'not_found',
          correlation_id: '00000000-0000-0000-0000-000000000000',
        }}
      />,
    )
    expect(
      screen.getByRole('heading', { name: 'craftPulse', level: 1 }),
    ).toBeInTheDocument()
    expect(screen.getByRole('alert')).toHaveTextContent('intentional failure')
  })
})
