import { describe, expect, it } from 'vitest'
import { createLoginUrl, safeReturnTo } from './return-to'

describe('return_to', () => {
  it.each([
    'https://evil.example/path',
    '//evil.example',
    '/\\evil.example',
    '/%5cevil.example',
    '/%2f%2fevil.example',
    '/%252f%252fevil.example',
    '/a/..//evil.example',
    '/%0aevil',
    '/a?b=%00',
    '',
    null,
    undefined,
    '/' + 'a'.repeat(2049),
  ])('rejects an unsafe value: %s', (value) => {
    expect(safeReturnTo(value)).toBe('/')
  })
  it('preserves query encoding and fragments on a safe same-origin path', () => {
    expect(safeReturnTo('/users/123?name=a%26b#roles')).toBe(
      '/users/123?name=a%26b#roles',
    )
  })
  it('uses the site root login endpoint rather than /api/v1', () => {
    const url = createLoginUrl(
      'example',
      '/organization/structure?team=123#form',
    )
    expect(url.startsWith('/o/example/login?')).toBe(true)
    expect(
      new URL(url, 'https://craftpulse.invalid').searchParams.get('return_to'),
    ).toBe('/organization/structure?team=123#form')
    expect(() => createLoginUrl('../evil', '/')).toThrow()
  })
})
