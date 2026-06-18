import { test as base, expect } from '@playwright/test';
import { PlaywrightAiFixture, type PlayWrightAiFixtureType } from '@midscene/web/playwright';
import 'dotenv/config';

const test = base.extend<PlayWrightAiFixtureType>(PlaywrightAiFixture({
  cache: true,
}));

test('Midscene can inspect Smara Parallel Agent UI through Playwright fixture', async ({ page, aiQuery, aiAssert }) => {
  await page.addInitScript(() => window.localStorage.setItem('smara_active_tab', 'parallel-tasks'));
  await page.goto('/', { waitUntil: 'domcontentloaded' });

  const appMain = page.locator('main');
  await expect(page).toHaveTitle(/Smara|Vite|React/i);
  await expect(page.locator('nav').getByText('Parallel Agent')).toBeVisible({ timeout: 15000 });
  await expect(appMain.getByRole('heading', { name: 'Parallel Agent' })).toBeVisible({ timeout: 15000 });
  await expect(appMain.locator('header').getByText('Parallel Agent Orchestration')).toBeVisible();
  await expect(appMain.locator('aside').getByText('Parallel Agent Config').last()).toBeVisible();
  await expect(appMain.getByText('Agent Network', { exact: true }).last()).toBeVisible();
  await expect(appMain.getByText('Coordinator Agent', { exact: true }).last()).toBeVisible();
  await expect(appMain.locator('aside').getByText('Current Run', { exact: true }).last()).toBeVisible();

  const summary = await aiQuery<string>('string, describe the Parallel Agent page, including whether it has connected agent nodes or an agent orchestration network');
  console.log('Midscene Parallel Agent summary:', summary);

  await aiAssert('the page is a Parallel Agent orchestration interface with agent-related controls and visible navigation');
});
