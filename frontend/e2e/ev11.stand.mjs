import { expect, test } from '@playwright/test'
import {
  activate,
  checkDialog,
  choose,
  noPageOverflow,
  tabTo,
} from './keyboard.mjs'
import { profiles, settings } from './settings.mjs'
import {
  editorVersion,
  ensureFixture,
  login,
  me,
  readStructure,
  replaceStructure,
} from './stand.mjs'

const run = settings()
const orgA = 'org-a'
const orgB = 'org-b'
const heading = (page, name) =>
  page.getByRole('heading', { level: 1, name, exact: true })
const button = (scope, name) => scope.getByRole('button', { name, exact: true })
const link = (scope, name) => scope.getByRole('link', { name, exact: true })
const select = (scope, name) =>
  scope.getByRole('combobox', { name, exact: true })

async function type(page, control, value) {
  await tabTo(page, control)
  await page.keyboard.press('ControlOrMeta+A')
  await page.keyboard.insertText(value)
}

function keyboardLogin(page, account, org) {
  return login(page, run, account, org, {
    activate: (control) => activate(page, control),
    type: (control, value) => type(page, control, value),
  })
}

test.beforeEach(async ({ browser, page, request }, info) => {
  const response = await request.get('/version', { maxRedirects: 0 })
  expect(response.ok()).toBe(true)
  expect((await response.json()).commit_sha).toBe(run.sha)
  expect(browser.version()).toBe(run.browserVersion)
  expect(page.viewportSize()).toEqual(profiles[run.profile].viewport)
  await info.attach('verified-browser', {
    body: browser.version(),
    contentType: 'text/plain',
  })
})

test.afterEach(async ({ request }) => {
  const response = await request.get('/version', { maxRedirects: 0 })
  expect(response.ok()).toBe(true)
  expect((await response.json()).commit_sha).toBe(run.sha)
})

// The dedicated snapshot has not completed the first login in org-b.
test('[01] UI-02, UI-03, UI-06: login, refusal and first setup', async ({
  page,
  browser,
}) => {
  await test.step('refused login and return to start', async () => {
    await keyboardLogin(page, 'outsider', orgB)
    await expect(heading(page, 'Не удалось войти')).toBeFocused()
    await expect(page.getByRole('alert')).toContainText(
      'Организация ещё не настроена',
    )
    await noPageOverflow(page)
    await page.keyboard.press('Tab')
    const start = button(page, 'Вернуться к началу')
    await expect(start).toBeFocused()
    await page.keyboard.press('Space')
    await expect(heading(page, 'Вход')).toBeFocused()
    await noPageOverflow(page)
  })

  await test.step('first administrator opens the setup result and structure', async () => {
    // Use a new context so GitLab does not reuse the outsider session.
    const context = await browser.newContext({
      baseURL: run.site,
      viewport: profiles[run.profile].viewport,
    })
    try {
      const admin = await context.newPage()
      await keyboardLogin(admin, 'adminB', orgB)
      await expect(heading(admin, 'Результат первого входа')).toBeFocused()
      await expect(
        admin.getByText('Вам назначена роль администратора'),
      ).toBeVisible()
      await noPageOverflow(admin)
      await activate(admin, link(admin, 'Описать структуру организации'))
      await editorVersion(admin)
      await expect(heading(admin, 'Структура организации')).toBeFocused()
      await noPageOverflow(admin)
    } finally {
      await context.close()
    }
  })
})

test('[02] UI-01, UI-04: scope, direct denial, installation and logout', async ({
  page,
}) => {
  await keyboardLogin(page, 'adminA', orgA)
  await me(page, run, orgA)
  const { team, project } = await ensureFixture(page, run, orgA)
  await page.goto('/')

  await test.step('skip link precedes the team and project selectors', async () => {
    const scope = page.getByRole('group', { name: 'Выбор области' })
    const teamSelect = select(scope, 'Команда')
    const projectSelect = select(scope, 'Проект')
    await expect(heading(page, 'Обзор')).toBeFocused()
    await expect(teamSelect).toBeEnabled()
    await tabTo(page, link(page, 'К содержимому'))
    await page.keyboard.press('Tab')
    await expect(teamSelect).toBeFocused()
    await choose(page, teamSelect, team.id)
    await expect(projectSelect).toBeEnabled()
    await expect(teamSelect).toBeFocused()
    await page.keyboard.press('Tab')
    await expect(projectSelect).toBeFocused()
    await choose(page, projectSelect, project.id)
    await expect(projectSelect).toBeFocused()
    await page.keyboard.press('Shift+Tab')
    await expect(teamSelect).toBeFocused()
    await noPageOverflow(page)
  })

  await test.step('open an unavailable direct link and return', async () => {
    await page.goto('/timeline?project_id=00000000-0000-4000-8000-000000000001')
    await expect(heading(page, 'Объект или область недоступны')).toBeFocused()
    await noPageOverflow(page)
    await activate(page, button(page, 'К списку'))
    await expect(heading(page, 'Обзор')).toBeFocused()
  })

  await test.step('open installation diagnostics and sign out', async () => {
    await activate(page, link(page, 'Диагностика установки'))
    await expect(heading(page, 'Диагностика установки')).toBeFocused()
    await expect(page.getByText('Общий исход', { exact: true })).toBeVisible()
    await noPageOverflow(page)
    await activate(page, button(page, 'Выход'), 'Space')
    await expect(heading(page, 'Вход')).toBeFocused()
    await noPageOverflow(page)
  })
})

test('[03] UI-08: role confirmation, ending sessions and blocking', async ({
  page,
  browser,
}) => {
  await keyboardLogin(page, 'adminA', orgA)
  const { team } = await ensureFixture(page, run, orgA)
  const context = await browser.newContext({
    baseURL: run.site,
    viewport: profiles[run.profile].viewport,
  })
  try {
    const member = await context.newPage()
    await keyboardLogin(member, 'member', orgA)
    const current = await me(member, run, orgA)

    await test.step('add an assignment and confirm it', async () => {
      await activate(page, link(page, 'Пользователи и роли'))
      await activate(
        page,
        page
          .getByRole('table', { name: 'Список пользователей' })
          .locator(`a[href="/users/${encodeURIComponent(current.user.id)}"]`),
      )
      const form = page.getByRole('form', { name: 'Роли и области' })
      await choose(page, select(form, 'Роль'), 'dev_lead')
      await choose(page, select(form, 'Область роли'), 'team_id')
      await choose(page, select(form, 'Команда'), team.id)
      await activate(page, button(form, 'Добавить назначение'), 'Space')
      const review = button(form, 'Просмотреть изменения ролей')
      const title = 'Подтвердите изменение ролей'
      const dialog = page.getByRole('dialog', { name: title })
      const confirm = button(dialog, 'Подтвердить')
      await activate(page, review)
      await checkDialog(
        page,
        dialog,
        review,
        button(dialog, 'Отмена'),
        confirm,
        dialog.getByRole('heading', { name: title, exact: true }),
      )
      await page.keyboard.press('Enter')
      await activate(page, confirm)
      await expect(form.getByText('Роли сохранены')).toBeVisible()
      await expect(heading(page, 'Пользователи и роли')).toBeFocused()
      await noPageOverflow(page)
    })

    for (const [label, result, nextControl] of [
      [
        'Завершить все сессии',
        'Все сессии пользователя завершены.',
        'Завершить все сессии',
      ],
      ['Заблокировать', 'Пользователь заблокирован.', 'Разблокировать'],
    ]) {
      await test.step(
        label === 'Заблокировать' ? 'block user' : 'end all sessions',
        async () => {
          const panel = page.getByRole('region', {
            name: 'Блокировка и сессии',
          })
          const opener = button(panel, label)
          const dialog = page.getByRole('dialog', { name: label })
          const confirm = button(dialog, label)
          await activate(page, opener)
          await checkDialog(
            page,
            dialog,
            opener,
            button(dialog, 'Отмена'),
            confirm,
          )
          await page.keyboard.press('Space')
          await activate(page, confirm, 'Space')
          await expect(panel.getByText(result, { exact: true })).toBeVisible()
          await expect(button(panel, nextControl)).toBeFocused()
          await noPageOverflow(page)
          if (label === 'Завершить все сессии') {
            await member.reload()
            await expect(heading(member, 'Сессия завершилась')).toBeFocused()
            await noPageOverflow(member)
          }
        },
      )
    }
  } finally {
    await context.close()
  }
})

test('[04] UI-07, UI-17, UI-18: validation, concurrence and unsaved changes', async ({
  page,
}) => {
  await keyboardLogin(page, 'adminA', orgA)
  const { team, project } = await ensureFixture(page, run, orgA)
  await activate(page, link(page, 'Структура организации'))
  const version = await editorVersion(page)
  const name = page.getByRole('textbox', { name: 'Имя', exact: true })
  const review = button(page, 'Просмотреть изменения')
  const dialog = page.getByRole('dialog', {
    name: 'Подтвердить структуру организации',
  })
  const confirm = button(dialog, 'Подтвердить')
  const title = dialog.getByRole('heading', {
    name: 'Подтвердить структуру организации',
    exact: true,
  })

  await test.step('resolve the error from its summary', async () => {
    await activate(page, button(page, 'Изменить ' + team.name))
    await expect(name).toBeFocused()
    await page.keyboard.press('ControlOrMeta+A')
    await page.keyboard.press('Backspace')
    await activate(page, review)
    const summary = page
      .getByRole('alert')
      .filter({ hasText: 'Исправьте ошибки в структуре' })
    await expect(summary).toBeFocused()
    await page.keyboard.press('Tab')
    await expect(summary.getByRole('link')).toBeFocused()
    await page.keyboard.press('Enter')
    await expect(name).toBeFocused()
    await expect(name).toHaveAttribute('aria-invalid', 'true')
    await expect(name).toHaveAccessibleDescription(/Укажите/)
    await noPageOverflow(page)
    await type(page, name, 'EV-11 updated team')
  })

  await test.step('confirmation traps focus and Escape restores the opener', async () => {
    await activate(page, review)
    await checkDialog(
      page,
      dialog,
      review,
      button(dialog, 'Отмена'),
      confirm,
      title,
    )
  })

  await test.step('review a conflict and confirm the rebased proposal', async () => {
    await activate(page, review)
    const snapshot = await readStructure(page, run)
    // A second write to the same resource advances the version after preview.
    await replaceStructure(page, run, snapshot, {
      projects: snapshot.data.projects.map((item) =>
        item.id === project.id
          ? { ...item, name: 'EV-11 concurrent project' }
          : item,
      ),
    })
    await activate(page, confirm)
    await expect(
      dialog.getByText('Данные изменились. Просмотрите актуальную версию', {
        exact: true,
      }),
    ).toBeVisible()
    await expect(
      dialog.getByText('Станет: EV-11 concurrent project', { exact: true }),
    ).toBeVisible()
    await noPageOverflow(page)
    await activate(page, confirm)
    await activate(page, button(dialog, 'Готово'))
    await expect(
      page.getByText('Сохранённая версия структуры: ' + (version + 2), {
        exact: true,
      }),
    ).toBeVisible()
    await expect(heading(page, 'Структура организации')).toBeFocused()
  })

  await test.step('cancel and then accept navigation away from the draft', async () => {
    await activate(page, button(page, 'Изменить EV-11 updated team'))
    await type(page, name, 'EV-11 discarded draft')
    const users = link(page, 'Пользователи и роли')
    const unsaved = page.getByRole('dialog', { name: 'Изменения не сохранены' })
    const keep = button(unsaved, 'Продолжить редактирование')
    const discard = button(unsaved, 'Отбросить изменения')
    await activate(page, users)
    await checkDialog(page, unsaved, users, keep, discard)
    await expect(name).toHaveValue('EV-11 discarded draft')
    await page.keyboard.press('Enter')
    await activate(page, discard, 'Space')
    await expect(heading(page, 'Пользователи и роли')).toBeFocused()
    await noPageOverflow(page)
  })
})
