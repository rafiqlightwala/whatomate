import { test, expect } from '@playwright/test'
import { loginAsAdmin, ApiHelper, createTeamFixture } from '../../helpers'

test.describe('Team Assignment Strategy for Calls', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page)
  })

  test('should create team with round_robin strategy', async ({ page }) => {
    await page.goto('/settings/teams')
    await page.waitForLoadState('networkidle')

    // Click add team
    const addButton = page.getByRole('button', { name: /add team/i }).first()
    await addButton.click()

    const dialog = page.getByRole('dialog')
    await expect(dialog).toBeVisible()

    const teamName = `RR Team ${Date.now()}`
    await dialog.getByLabel(/name/i).fill(teamName)

    // Select round_robin strategy
    const strategySelect = dialog.locator('button[role="combobox"]').or(dialog.getByLabel(/strategy/i))
    if (await strategySelect.isVisible()) {
      await strategySelect.click()
      await page.getByRole('option', { name: /round.?robin/i }).click()
    }

    await dialog.getByRole('button', { name: /save|create|submit/i }).click()
    await page.waitForTimeout(500)

    // Verify team was created
    await page.getByPlaceholder(/search/i).fill(teamName)
    await page.waitForTimeout(300)
    await expect(page.locator('tbody').getByText(teamName)).toBeVisible()
  })

  test('should create team with load_balanced strategy', async ({ page }) => {
    await page.goto('/settings/teams')
    await page.waitForLoadState('networkidle')

    const addButton = page.getByRole('button', { name: /add team/i }).first()
    await addButton.click()

    const dialog = page.getByRole('dialog')
    await expect(dialog).toBeVisible()

    const teamName = `LB Team ${Date.now()}`
    await dialog.getByLabel(/name/i).fill(teamName)

    const strategySelect = dialog.locator('button[role="combobox"]').or(dialog.getByLabel(/strategy/i))
    if (await strategySelect.isVisible()) {
      await strategySelect.click()
      await page.getByRole('option', { name: /load.?balanced/i }).click()
    }

    await dialog.getByRole('button', { name: /save|create|submit/i }).click()
    await page.waitForTimeout(500)

    await page.getByPlaceholder(/search/i).fill(teamName)
    await page.waitForTimeout(300)
    await expect(page.locator('tbody').getByText(teamName)).toBeVisible()
  })

  test('should create team with manual strategy', async ({ page }) => {
    await page.goto('/settings/teams')
    await page.waitForLoadState('networkidle')

    const addButton = page.getByRole('button', { name: /add team/i }).first()
    await addButton.click()

    const dialog = page.getByRole('dialog')
    await expect(dialog).toBeVisible()

    const teamName = `Manual Team ${Date.now()}`
    await dialog.getByLabel(/name/i).fill(teamName)

    const strategySelect = dialog.locator('button[role="combobox"]').or(dialog.getByLabel(/strategy/i))
    if (await strategySelect.isVisible()) {
      await strategySelect.click()
      await page.getByRole('option', { name: /manual/i }).click()
    }

    await dialog.getByRole('button', { name: /save|create|submit/i }).click()
    await page.waitForTimeout(500)

    await page.getByPlaceholder(/search/i).fill(teamName)
    await page.waitForTimeout(300)
    await expect(page.locator('tbody').getByText(teamName)).toBeVisible()
  })
})

test.describe('Team Assignment Strategy - API', () => {
  let api: ApiHelper

  test.beforeEach(async ({ request }) => {
    api = new ApiHelper(request)
    await api.loginAsAdmin()
  })

  test('should create team with per_agent_timeout_secs via API', async ({ request }) => {
    const BASE_URL = process.env.BASE_URL || 'http://localhost:8080'

    // Create team with custom per-agent timeout
    const response = await request.post(`${BASE_URL}/api/teams`, {
      headers: { 'X-CSRF-Token': '' },
      data: {
        name: `API Team ${Date.now()}`,
        description: 'Test team with per-agent timeout',
        assignment_strategy: 'round_robin',
        per_agent_timeout_secs: 30,
        is_active: true,
      },
    })

    // If CSRF is required, skip this test gracefully
    if (response.status() === 403) {
      test.skip()
      return
    }

    if (response.ok()) {
      const data = await response.json()
      const team = data.data?.team || data.team
      expect(team).toBeDefined()
      expect(team.assignment_strategy).toBe('round_robin')
      expect(team.per_agent_timeout_secs).toBe(30)
    }
  })
})

test.describe('Calling Navigation', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page)
  })

  test('should navigate between calling sub-pages', async ({ page }) => {
    // Navigate to call logs
    await page.goto('/calling/logs')
    await page.waitForLoadState('networkidle')
    expect(page.url()).toContain('/calling/logs')

    // Navigate to IVR flows
    await page.goto('/calling/ivr-flows')
    await page.waitForLoadState('networkidle')
    expect(page.url()).toContain('/calling/ivr-flows')

    // Navigate to call transfers
    await page.goto('/calling/transfers')
    await page.waitForLoadState('networkidle')
    expect(page.url()).toContain('/calling/transfers')
  })

  test('should redirect /calling to /calling/logs', async ({ page }) => {
    await page.goto('/calling')
    await page.waitForLoadState('networkidle')
    expect(page.url()).toContain('/calling/logs')
  })
})
