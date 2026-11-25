import { describe, expect, it } from 'vitest'
import { changeScope } from '../model/scope'

describe('UI-01 scope changes', () => {
  it('does not carry a project to another team', () => {
    const { params, reset } = changeScope(
      new URLSearchParams('team_id=old&project_id=project&view=overview'),
      'new',
    )
    expect(params.toString()).toBe('view=overview&team_id=new')
    expect(reset).toBe(true)
  })
})
