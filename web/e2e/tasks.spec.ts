import { test, expect } from '@playwright/test';

test.describe('Task Board', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    // Navigate to Tasks tab
    await page.getByRole('button', { name: 'Tasks' }).click();
  });

  test('"New Project" button exists', async ({ page }) => {
    const newProjectButton = page.getByRole('button', { name: /New Project/ });
    await expect(newProjectButton).toBeVisible();
  });

  test('clicking "New Project" opens a form', async ({ page }) => {
    await page.getByRole('button', { name: /New Project/ }).click();

    // The project form should now be visible with input fields
    await expect(page.getByPlaceholder('Project name')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Create Project' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Cancel' })).toBeVisible();
  });

  test('project form has required fields', async ({ page }) => {
    await page.getByRole('button', { name: /New Project/ }).click();

    // Verify name and description fields are present
    const nameInput = page.getByPlaceholder('Project name');
    const descriptionInput = page.getByPlaceholder('Description (optional)');

    await expect(nameInput).toBeVisible();
    await expect(descriptionInput).toBeVisible();

    // Create Project button should be disabled when required fields are empty
    const createButton = page.getByRole('button', { name: 'Create Project' });
    await expect(createButton).toBeDisabled();
  });
});
