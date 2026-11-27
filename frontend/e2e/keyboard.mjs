import { expect } from '@playwright/test'

// Navigation and activation use the current document focus. Locator.press()
// would silently focus its target even when the target is unreachable by Tab.
export async function tabTo(page, control, direction = 'Tab') {
  await expect(control).toBeVisible()
  for (let step = 0; step < 160; step++) {
    if (
      await control.evaluate(
        (element) => element === element.ownerDocument.activeElement,
      )
    )
      return
    await page.keyboard.press(direction)
  }
  await expect(control).toBeFocused()
}

export async function activate(page, control, key = 'Enter') {
  await tabTo(page, control)
  await page.keyboard.press(key)
}

export async function choose(page, control, value) {
  await expect(control).toBeEnabled()
  await tabTo(page, control)
  const { selected, desired } = await control.evaluate(
    (select, value) => ({
      selected: select.selectedIndex,
      desired: Array.from(select.options).findIndex(
        (option) => option.value === value,
      ),
    }),
    value,
  )
  expect(desired, 'Requested select option').toBeGreaterThanOrEqual(0)
  const key = desired > selected ? 'ArrowDown' : 'ArrowUp'
  for (let index = 0; index < Math.abs(desired - selected); index++) {
    await expect(control).toBeFocused()
    await page.keyboard.press(key)
  }
  await expect(control).toHaveValue(value)
}

export async function noPageOverflow(page) {
  await expect
    .poll(() =>
      page
        .locator('html')
        .evaluate(
          (root) =>
            Math.max(root.scrollWidth, root.ownerDocument.body.scrollWidth) -
            root.clientWidth,
        ),
    )
    .toBeLessThanOrEqual(0)
}

export async function checkDialog(
  page,
  dialog,
  opener,
  first,
  last,
  initial = first,
) {
  await expect(dialog).toBeVisible()
  await expect(initial).toBeFocused()
  await noPageOverflow(page)
  await page.keyboard.press('Shift+Tab')
  await expect(last).toBeFocused()
  await page.keyboard.press('Tab')
  await expect(first).toBeFocused()
  await page.keyboard.press('Tab')
  await expect(last).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(dialog).toBeHidden()
  await expect(opener).toBeFocused()
}
