import { test, expect } from '@playwright/test';

test.describe('Activity Feed', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
  });

  test('shows empty state when no activity exists', async ({ page }) => {
    await expect(
      page.getByText('No activity yet — waiting for events…')
    ).toBeVisible();
  });

  test('connection status indicator is visible', async ({ page }) => {
    // The connection indicator is a colored dot with a title attribute
    const indicator = page.locator('[title="Connected (SSE)"], [title="Disconnected (polling)"]');
    await expect(indicator.first()).toBeVisible();
  });

  test('pause/resume button exists and toggles', async ({ page }) => {
    const pauseButton = page.getByRole('button', { name: /Pause/ });
    await expect(pauseButton).toBeVisible();

    // Click to toggle to Resume
    await pauseButton.click();
    const resumeButton = page.getByRole('button', { name: /Resume/ });
    await expect(resumeButton).toBeVisible();

    // Click to toggle back to Pause
    await resumeButton.click();
    await expect(page.getByRole('button', { name: /Pause/ })).toBeVisible();
  });
});
