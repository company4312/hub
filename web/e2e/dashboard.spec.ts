import { test, expect } from '@playwright/test';

test.describe('Dashboard', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
  });

  test('loads and shows Activity tab as default', async ({ page }) => {
    const activityTab = page.getByRole('button', { name: 'Activity' });
    await expect(activityTab).toBeVisible();
  });

  test('renders the agent sidebar', async ({ page }) => {
    await expect(page.getByText('Company4312')).toBeVisible();
    await expect(page.getByText('Agent Dashboard')).toBeVisible();
    await expect(page.getByText('All Activity')).toBeVisible();
  });

  test('switches between Activity, Memories, and Tasks tabs', async ({ page }) => {
    // Activity tab is visible by default
    const activityTab = page.getByRole('button', { name: 'Activity' });
    const tasksTab = page.getByRole('button', { name: 'Tasks' });

    await expect(activityTab).toBeVisible();
    await expect(tasksTab).toBeVisible();

    // Switch to Tasks tab
    await tasksTab.click();
    await expect(page.getByText('Backlog')).toBeVisible();

    // Switch back to Activity tab
    await activityTab.click();
    await expect(page.getByText('All Activity')).toBeVisible();
  });

  test('Tasks tab shows Kanban board columns', async ({ page }) => {
    const tasksTab = page.getByRole('button', { name: 'Tasks' });
    await tasksTab.click();

    // Verify all Kanban columns are rendered
    await expect(page.getByText('Backlog')).toBeVisible();
    await expect(page.getByText('Todo')).toBeVisible();
    await expect(page.getByText('In Progress')).toBeVisible();
    await expect(page.getByText('Review')).toBeVisible();
    await expect(page.getByText('Done')).toBeVisible();
  });
});
