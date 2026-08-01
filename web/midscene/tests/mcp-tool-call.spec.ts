import { test, expect } from '@playwright/test';
import 'dotenv/config';

test('Smara Web Chat - MCP Tool Call with Prefix Test', async ({ page }) => {
  await page.addInitScript(() => window.localStorage.setItem('smara_active_tab', 'chat'));
  await page.goto('http://127.0.0.1:8080', { waitUntil: 'domcontentloaded' });

  // Verify page loaded
  await expect(page).toHaveTitle(/Smara/i);
  
  // Find chat textarea or input
  const textarea = page.locator('textarea').first();
  await expect(textarea).toBeVisible({ timeout: 15000 });

  // Type prompt requesting MCP tool call with prefix
  const testPrompt = 'tolong periksa status index codebase-memory-mcp:index_repository pada repo ini';
  await textarea.fill(testPrompt);

  // Click send button or press Enter
  const sendButton = page.locator('button').filter({ has: page.locator('svg') }).last();
  await textarea.press('Enter');

  console.log('Prompt sent:', testPrompt);

  // Wait for response to stream and complete
  await page.waitForTimeout(8000);

  // Capture screenshot of the chat UI
  await page.screenshot({ path: 'mcp-tool-test-result.png', fullPage: true });

  // Assert that there is no "tidak ditemukan di route map" error on page
  const pageContent = await page.content();
  expect(pageContent).not.toContain('tidak ditemukan di route map');
  console.log('✅ Real Playwright Test passed: No route map error detected!');
});
