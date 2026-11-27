import { expect, test } from '@playwright/test'
import { profiles, settings } from './settings.mjs'
import {
  api,
  confirm,
  editorVersion,
  ensureFixture,
  gitlabFlow,
  login,
  me,
  noMarkers,
  readStructure,
  rename,
  reviewStructure,
  structurePath,
} from './stand.mjs'

const run = settings()
const orgA = 'org-a'
const orgB = 'org-b'
const memberPage = '/users'
const newContext = (browser) =>
  browser.newContext({
    baseURL: run.site,
    viewport: profiles[run.profile].viewport,
  })
const sessionEnded = (page) =>
  page.getByRole('heading', { name: 'Сессия завершилась' })
const scopePicker = (page) =>
  page.getByRole('group', { name: 'Выбор области', exact: true })
const teamSelect = (page) =>
  scopePicker(page).getByRole('combobox', { name: 'Команда', exact: true })
const projectSelect = (page) =>
  scopePicker(page).getByRole('combobox', { name: 'Проект', exact: true })
const usersLink = (page) =>
  page.getByRole('link', { name: 'Пользователи и роли', exact: true })

function apiResponse(response, method, path) {
  const url = new URL(response.url())
  return (
    url.origin === run.site &&
    url.pathname === '/api/v1' + path &&
    response.request().method() === method
  )
}

test.beforeEach(async ({ browser, request }, info) => {
  await test.step('verify release and browser', async () => {
    const response = await request.get('/version', { maxRedirects: 0 })
    expect(response.ok()).toBe(true)
    expect((await response.json()).commit_sha).toBe(run.sha)
    expect(browser.version()).toBe(run.browserVersion)
    await info.attach('verified-browser', {
      body: browser.version(),
      contentType: 'text/plain',
    })
  })
})

test.afterEach(async ({ request }) => {
  await test.step('verify release after scenario', async () => {
    const response = await request.get('/version', { maxRedirects: 0 })
    expect(response.ok()).toBe(true)
    expect((await response.json()).commit_sha).toBe(run.sha)
  })
})

// The full run starts from the pre-bootstrap snapshot. Scenario 03 reads org-b
// after this scenario has verified that its first administrator can activate it.
test('[05] first administrator and setup rejection', async ({
  page,
  browser,
}) => {
  await test.step('reject outsider before bootstrap', async () => {
    const outsider = await newContext(browser)
    try {
      const denied = await outsider.newPage()
      await login(denied, run, 'outsider', orgB)
      await expect(
        denied.getByText('Организация ещё не настроена', { exact: true }),
      ).toBeVisible()
      await expect(denied.getByRole('group', { name: 'Профиль' })).toHaveCount(
        0,
      )
    } finally {
      await outsider.close()
    }
  })
  await test.step('bootstrap with the declared administrator', async () => {
    await login(page, run, 'adminB', orgB)
    await expect(
      page.getByRole('heading', { name: 'Результат первого входа' }),
    ).toBeVisible()
    const current = await me(page, run, orgB)
    expect(current.roles.map(({ role }) => role)).toEqual(['administrator'])
    await page
      .getByRole('link', { name: 'Описать структуру организации' })
      .click()
    await editorVersion(page)
  })
})

test('[01] GitLab login and scope', async ({ page }) => {
  const { current, team, project } =
    await test.step('sign in and seed the AC-01 fixture', async () => {
      await login(page, run, 'adminA', orgA)
      const current = await me(page, run, orgA)
      return { current, ...(await ensureFixture(page, run, orgA)) }
    })
  await test.step('open only the first organization scopes', async () => {
    await page.goto('/')
    await expect(
      page.getByText(current.organization.name, { exact: true }).first(),
    ).toBeVisible()
    await expect(teamSelect(page).locator('option:not([value=""])')).toHaveText(
      [team.name],
    )
    await teamSelect(page).selectOption(team.id)
    await expect(
      projectSelect(page).locator('option:not([value=""])'),
    ).toHaveText([project.name])
    await projectSelect(page).selectOption(project.id)
    await expect(projectSelect(page)).toHaveValue(project.id)
  })
})

test('[02] old tab after logout', async ({ page, context }) => {
  const { current, tab, fixture } =
    await test.step('open organization data in two tabs', async () => {
      await login(page, run, 'adminA', orgA)
      const current = await me(page, run, orgA)
      const fixture = await ensureFixture(page, run, orgA)
      const tab = await context.newPage()
      await tab.goto(structurePath)
      await editorVersion(tab)
      await expect(
        tab.getByRole('cell', { name: fixture.team.name }),
      ).toBeVisible()
      return { current, tab, fixture }
    })
  await test.step('sign out and return to the old tab', async () => {
    await page.bringToFront()
    await page.getByRole('button', { name: 'Выход', exact: true }).click()
    await expect(
      page.getByRole('heading', { name: 'Вход', exact: true }),
    ).toBeVisible()
    await tab.bringToFront()
    // Focus may trigger UI-03 before the navigation link can be clicked.
    await Promise.any([
      sessionEnded(tab).waitFor({ state: 'visible', timeout: 15_000 }),
      usersLink(tab).click({ timeout: 15_000 }),
    ])
    await expect(sessionEnded(tab)).toBeVisible()
    await noMarkers(tab, [
      current.organization.name,
      fixture.team.name,
      fixture.project.name,
    ])
    await expect(tab.getByRole('group', { name: 'Профиль' })).toHaveCount(0)
  })
})

test('[03] foreign project and team links', async ({ page, browser }) => {
  const { foreign, markers } =
    await test.step('resolve actual foreign IDs in a separate context', async () => {
      const other = await newContext(browser)
      try {
        const foreignPage = await other.newPage()
        await login(foreignPage, run, 'adminB', orgB)
        const current = await me(foreignPage, run, orgB)
        const foreign = await ensureFixture(foreignPage, run, orgB)
        const extras = JSON.parse(process.env.AC01_FOREIGN_MARKERS ?? '[]')
        expect(
          Array.isArray(extras) &&
            extras.every(
              (value) => typeof value === 'string' && value.length > 0,
            ),
        ).toBe(true)
        return {
          foreign,
          markers: [
            current.organization.name,
            foreign.team.name,
            foreign.project.name,
            foreign.team.id,
            foreign.project.id,
            ...extras,
          ],
        }
      } finally {
        await other.close()
      }
    })
  await test.step('open foreign links directly and inspect the DOM', async () => {
    await login(page, run, 'adminA', orgA)
    await me(page, run, orgA)
    for (const path of [
      `/timeline?project_id=${foreign.project.id}`,
      `/reports/team?team_id=${foreign.team.id}`,
    ]) {
      const response = await page.goto(path)
      expect(response?.ok()).toBe(true)
      await expect(
        page.getByRole('heading', { name: 'Объект или область недоступны' }),
      ).toBeVisible()
      await noMarkers(page, markers)
    }
  })
})

async function secondAdministrator(page, browser) {
  const context = await newContext(browser)
  try {
    const second = await context.newPage()
    await login(second, run, 'adminB', orgA)
    const user = await me(second, run, orgA)
    const path = `/users/${user.user.id}/roles`
    const assignment = {
      role: 'administrator',
      team_id: null,
      project_id: null,
      service_id: null,
    }
    const roles = await api(page, run, path)
    if (!roles.data.data.length) {
      expect(roles.tag).toBeTruthy()
      await api(page, run, path, {
        method: 'PUT',
        tag: roles.tag,
        body: { data: [assignment] },
      })
    } else {
      expect(roles.data.data).toMatchObject([assignment])
    }
    return { context, second, user }
  } catch (error) {
    await context.close()
    throw error
  }
}

test('[04] expired session and return without form replay', async ({
  page,
  browser,
}) => {
  const { team } = await test.step('prepare two administrators', async () => {
    await login(page, run, 'adminA', orgA)
    await me(page, run, orgA)
    return ensureFixture(page, run, orgA)
  })
  const { context, second, user } =
    await test.step('open a second administrator session', () =>
      secondAdministrator(page, browser))
  try {
    await test.step('end sessions while a form is open', async () => {
      await second.goto(structurePath)
      const version = await editorVersion(second)
      await rename(second, team.name, 'AC-01 unsent draft')
      const url = second.url()
      const dialog = await reviewStructure(second)
      await api(page, run, `/users/${user.user.id}/sessions`, {
        method: 'DELETE',
        status: 204,
      })

      const [response] = await Promise.all([
        second.waitForResponse((response) =>
          apiResponse(response, 'PUT', structurePath),
        ),
        confirm(dialog),
      ])
      expect(response.status()).toBe(401)
      expect((await response.json()).code).toBe('auth_required')
      await expect(sessionEnded(second)).toBeVisible()
      await noMarkers(second, [team.name])
      let replays = 0
      second.on('request', (request) => {
        if (
          request.method() === 'PUT' &&
          new URL(request.url()).pathname === '/api/v1' + structurePath
        )
          replays++
      })

      await test.step('sign in again and verify no replay', async () => {
        await second
          .getByRole('button', { name: 'Войти снова', exact: true })
          .click()
        await gitlabFlow(second, run, 'adminB')
        await expect(second).toHaveURL(url)
        expect(await editorVersion(second)).toBe(version)
        expect((await readStructure(page, run)).data.teams).toMatchObject([
          { name: team.name },
        ])
        expect(replays).toBe(0)
      })
    })
  } finally {
    await context.close()
  }
})

test('[07] scoped assignment is visible on the next request', async ({
  page,
  browser,
}) => {
  const { team, project } =
    await test.step('prepare the administrator and team', async () => {
      await login(page, run, 'adminA', orgA)
      await me(page, run, orgA)
      return ensureFixture(page, run, orgA)
    })
  const context = await newContext(browser)
  try {
    const member = await context.newPage()
    const session = async () =>
      (await context.cookies(run.site)).find(
        ({ name }) => name === '__Host-cp_session',
      )?.value
    const { current, cookie } =
      await test.step('open the member session before assignment', async () => {
        await login(member, run, 'member', orgA)
        const current = await me(member, run, orgA)
        expect(current.roles).toEqual([])
        expect(current.areas.teams).toEqual([])
        await expect(scopePicker(member)).toHaveCount(0)
        const cookie = await session()
        expect(Boolean(cookie)).toBe(true)
        return { current, cookie }
      })
    await test.step('assign a team role through UI-08', async () => {
      await page.goto(memberPage + '/' + current.user.id)
      const form = page.getByRole('form', { name: 'Роли и области' })
      await form
        .getByRole('combobox', { name: 'Роль', exact: true })
        .selectOption('dev_lead')
      await form
        .getByRole('combobox', { name: 'Область роли', exact: true })
        .selectOption('team_id')
      await form
        .getByRole('combobox', { name: 'Команда', exact: true })
        .selectOption(team.id)
      await form
        .getByRole('button', { name: 'Добавить назначение', exact: true })
        .click()
      await page
        .getByRole('button', {
          name: 'Просмотреть изменения ролей',
          exact: true,
        })
        .click()
      const dialog = page.getByRole('dialog', {
        name: 'Подтвердите изменение ролей',
      })
      const [response] = await Promise.all([
        page.waitForResponse((response) =>
          apiResponse(response, 'PUT', `/users/${current.user.id}/roles`),
        ),
        confirm(dialog),
      ])
      expect(response.ok()).toBe(true)
      await expect(
        form.getByText('Роли сохранены', { exact: true }),
      ).toBeVisible()
    })
    await test.step('see the new scope without another login', async () => {
      const [response] = await Promise.all([
        member.waitForResponse((response) =>
          apiResponse(response, 'GET', '/me'),
        ),
        member.goto('/'),
      ])
      expect(response.ok()).toBe(true)
      const data = await response.json()
      expect(data.roles).toMatchObject([{ role: 'dev_lead', team_id: team.id }])
      await expect(
        teamSelect(member).locator('option:not([value=""])'),
      ).toHaveText([team.name])
      await teamSelect(member).selectOption(team.id)
      await expect(
        projectSelect(member).locator('option:not([value=""])'),
      ).toHaveText([project.name])
      // Compare as a boolean so assertion diagnostics never interpolate session values.
      expect((await session()) === cookie).toBe(true)
    })
  } finally {
    await context.close()
  }
})

test('[06] structure conflict and unsaved changes', async ({
  page,
  browser,
}) => {
  const { team, project, second, context } =
    await test.step('prepare separate administrator contexts', async () => {
      await login(page, run, 'adminA', orgA)
      await me(page, run, orgA)
      const fixture = await ensureFixture(page, run, orgA)
      return { ...fixture, ...(await secondAdministrator(page, browser)) }
    })
  const firstName = 'AC-01 first confirmed team'
  const secondName = 'AC-01 second confirmed project'
  try {
    const version =
      await test.step('open the same version and prepare two drafts', async () => {
        const response = await page.goto(structurePath)
        expect(response?.ok()).toBe(true)
        expect(response?.headers()['content-type']).toContain('text/html')
        await second.goto(structurePath)
        const version = await editorVersion(page)
        expect(await editorVersion(second)).toBe(version)
        await rename(page, team.name, firstName)
        await rename(second, project.name, secondName, 'Проекты')
        return version
      })
    await test.step('resolve a stale version explicitly', async () => {
      const stale = await reviewStructure(second)
      await expect(stale).toBeVisible()
      await confirm(await reviewStructure(page))
      await expect(
        page.getByText(`Сохранённая версия структуры: ${version + 1}`),
      ).toBeVisible()
      const [conflict] = await Promise.all([
        second.waitForResponse((response) =>
          apiResponse(response, 'PUT', structurePath),
        ),
        confirm(stale),
      ])
      expect(conflict.status()).toBe(412)
      expect((await conflict.json()).code).toBe('version_conflict')
      await expect(
        stale.getByText('Данные изменились. Просмотрите актуальную версию', {
          exact: true,
        }),
      ).toBeVisible()
      await expect(
        stale.getByText('Станет: ' + firstName, { exact: true }),
      ).toBeVisible()
      const interim = await readStructure(page, run)
      expect(interim.data.version).toBe(version + 1)
      expect(interim.data.teams).toMatchObject([{ name: firstName }])
      expect(interim.data.projects).toMatchObject([{ name: project.name }])
      await confirm(stale)
      await stale.getByRole('button', { name: 'Готово', exact: true }).click()
      await expect(
        second.getByText(`Сохранённая версия структуры: ${version + 2}`),
      ).toBeVisible()
      const saved = await readStructure(page, run)
      expect(saved.data.teams).toMatchObject([{ name: firstName }])
      expect(saved.data.projects).toMatchObject([{ name: secondName }])
    })
    await test.step('confirm navigation away from an unsaved draft', async () => {
      await rename(second, secondName, 'AC-01 discarded draft')
      await usersLink(second).click()
      const dialog = second.getByRole('dialog', {
        name: 'Изменения не сохранены',
      })
      await expect(dialog).toBeVisible()
      await dialog
        .getByRole('button', { name: 'Отбросить изменения', exact: true })
        .click()
      await expect(
        second.getByRole('heading', { name: 'Список пользователей' }),
      ).toBeVisible()
      expect((await readStructure(page, run)).data.version).toBe(version + 2)
    })
  } finally {
    await context.close()
  }
})
