import { expect } from '@playwright/test'
import { accounts, password } from './settings.mjs'

export const structurePath = '/organization/structure'
const collections = [
  'teams',
  'projects',
  'services',
  'components',
  'team_projects',
  'project_areas',
  'team_members',
]

export function fixtureNames(org) {
  return { team: `AC-01 ${org} team`, project: `AC-01 ${org} project` }
}

export async function gitlabFlow(page, run, account) {
  const button = page.getByRole('button', { name: 'Войти через GitLab' })
  await expect(button).toBeVisible()
  const entry = page.url()
  await button.click()
  const signIn = (url) =>
    url.origin === run.gitlab && url.pathname === '/users/sign_in'
  const consent = (url) =>
    url.origin === run.gitlab && url.pathname === '/oauth/authorize'
  const returned = (url) =>
    url.origin === run.site &&
    url.href !== entry &&
    !/\/(login|callback)$/.test(url.pathname)
  await page.waitForURL((url) => signIn(url) || consent(url) || returned(url))

  if (signIn(new URL(page.url()))) {
    const form = page
      .locator('form')
      .filter({ has: page.locator('input[name="user[password]"]') })
    // Check the recipient before typing into the provider form.
    const action = new URL(
      (await form.getAttribute('action')) ?? '',
      page.url(),
    )
    const method = await form.getAttribute('method')
    expect(
      signIn(action) && method?.toLowerCase() === 'post',
      'GitLab sign-in form action',
    ).toBe(true)
    await form.locator('input[name="user[login]"]').fill(accounts[account])
    await form.locator('input[name="user[password]"]').fill(password(account))
    await form.locator('button[type="submit"], input[type="submit"]').click()
    await page.waitForURL((url) => consent(url) || returned(url))
  }

  if (consent(new URL(page.url()))) {
    // A redirect can pass through /oauth/authorize without rendering a consent form.
    const authorize = page.getByRole('button', {
      name: 'Authorize',
      exact: true,
    })
    const action = await Promise.race([
      authorize.waitFor().then(() => 'consent'),
      page.waitForURL(returned).then(() => 'returned'),
    ])
    if (action === 'consent') await authorize.click()
  }
  await page.waitForURL(returned)
}

export async function login(page, run, account, org) {
  await page.goto('/o/' + org)
  await gitlabFlow(page, run, account)
}

export async function me(page, run, org) {
  await expect(page.getByRole('group', { name: 'Профиль' })).toBeVisible()
  const { data } = await api(page, run, '/me')
  expect(data.organization.slug).toBe(org)
  expect(data.organization.status).toBe('active')
  return data
}

export async function api(
  page,
  run,
  path,
  { status = 200, method = 'GET', body, tag } = {},
) {
  const headers = method === 'GET' ? {} : { Origin: run.site }
  if (tag) headers['If-Match'] = tag
  const response = await page.request.fetch(run.site + '/api/v1' + path, {
    method,
    headers,
    data: body,
    maxRedirects: 0,
  })
  expect(response.status(), 'craftPulse API status').toBe(status)
  return {
    data: status === 204 ? undefined : await response.json(),
    tag: response.headers().etag,
  }
}

export async function readStructure(page, run) {
  const state = await api(page, run, structurePath)
  expect(Number.isSafeInteger(state.data.version)).toBe(true)
  expect(state.tag).toBe('"' + state.data.version + '"')
  return state
}

async function replaceStructure(page, run, state, changes) {
  const body = Object.fromEntries(
    collections.map((key) => [key, state.data[key]]),
  )
  await api(page, run, structurePath, {
    method: 'PUT',
    body: { ...body, ...changes },
    tag: state.tag,
  })
  return readStructure(page, run)
}

export async function ensureFixture(page, run, org) {
  let state = await readStructure(page, run)
  const names = fixtureNames(org)
  for (const key of collections
    .slice(2)
    .filter((key) => key !== 'team_projects')) {
    expect(state.data[key], 'Expected dedicated AC-01 structure').toEqual([])
  }
  for (const [key, name] of [
    ['teams', names.team],
    ['projects', names.project],
  ]) {
    if (state.data[key].length) {
      expect(state.data[key]).toMatchObject([{ name, status: 'active' }])
    }
  }

  const { teams, projects, team_projects: links } = state.data
  if (!teams.length || !projects.length) {
    expect(links).toEqual([])
    state = await replaceStructure(page, run, state, {
      teams: teams.length
        ? teams
        : [{ id: null, name: names.team, status: 'active' }],
      projects: projects.length
        ? projects
        : [{ id: null, name: names.project, status: 'active' }],
    })
  }
  const team = state.data.teams[0]
  const project = state.data.projects[0]
  const link = { team_id: team.id, project_id: project.id }
  if (state.data.team_projects.length === 0) {
    state = await replaceStructure(page, run, state, {
      team_projects: [{ ...link, valid_from: null, valid_to: null }],
    })
  }
  expect(state.data.team_projects).toMatchObject([link])
  return { team, project }
}

export async function editorVersion(page) {
  await expect(
    page.getByRole('heading', { name: 'Структура организации' }),
  ).toBeVisible()
  const version = page.getByText(/^Сохранённая версия структуры: \d+$/)
  await expect(version).toBeVisible()
  return Number((await version.innerText()).split(': ')[1])
}

export async function rename(page, from, to, section) {
  if (section)
    await page.getByRole('button', { name: section, exact: true }).click()
  await page
    .getByRole('button', { name: 'Изменить ' + from, exact: true })
    .click()
  await page.getByRole('textbox', { name: 'Имя', exact: true }).fill(to)
}

export async function reviewStructure(page) {
  await page
    .getByRole('button', { name: 'Просмотреть изменения', exact: true })
    .click()
  return page.getByRole('dialog', { name: 'Подтвердить структуру организации' })
}

export function confirm(dialog) {
  return dialog
    .getByRole('button', { name: 'Подтвердить', exact: true })
    .click()
}

export async function noMarkers(page, markers) {
  // Numeric markers describe displayed data, including hidden text. Avoid
  // matching incidental digits in SVG paths, element IDs or style values.
  const labels = []
  const numbers = []
  for (const marker of markers) {
    if (/^[+-−]?\p{N}[\p{N}\s.,]*%?$/u.test(marker)) {
      numbers.push('(?<!\\p{N})' + RegExp.escape(marker) + '(?!\\p{N})')
    } else {
      labels.push(marker)
    }
  }
  const found = await page.locator('body').evaluate(
    (body, { labels, numbers }) => {
      const markup = body.innerHTML
      const text = body.textContent ?? ''
      return (
        labels.some(
          (value) => markup.includes(value) || text.includes(value),
        ) || numbers.some((pattern) => new RegExp(pattern, 'u').test(text))
      )
    },
    { labels, numbers },
  )
  expect(found, 'Foreign or signed-out data in DOM').toBe(false)
}
