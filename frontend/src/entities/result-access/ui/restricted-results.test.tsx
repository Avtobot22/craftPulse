import { render, screen } from '@testing-library/react'
import { expect, it } from 'vitest'
import { RestrictedResults } from '..'

it.each([
  { redactions: ['location', 'evidence'] },
  { access_notes: [{ code: 'source_access_lost' }] },
  { access_limited: true },
])('shows a generic restriction message for %o', (props) => {
  render(<RestrictedResults {...props} />)
  expect(
    screen.getByText('Есть закрытые результаты.', { exact: true }),
  ).toBeVisible()
})

it('omits the message when there are no restrictions', () => {
  render(<RestrictedResults redactions={[]} access_notes={[]} />)
  expect(
    screen.queryByText('Есть закрытые результаты.'),
  ).not.toBeInTheDocument()
})
