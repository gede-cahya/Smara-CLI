const { chromium } = require('playwright');

(async () => {
  console.log('=== E2E Test: Smara Web UI Scheduler Dashboard ===');
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();

  console.log('Navigating to Smara Web (http://127.0.0.1:8080)...');
  await page.goto('http://127.0.0.1:8080');
  await page.waitForTimeout(2000);

  console.log('Locating and clicking "Scheduler" in sidebar...');
  const schedulerBtn = page.locator('text=Scheduler').first();
  await schedulerBtn.click();
  await page.waitForTimeout(2000);

  console.log('Verifying Scheduler Dashboard header...');
  const content = await page.textContent('body');
  if (content.includes('Scheduler & Cronjob Dashboard') || content.includes('Tambah Jadwal')) {
    console.log('✓ Scheduler Dashboard loaded successfully!');
  } else {
    console.error('❌ Scheduler Dashboard failed to load');
  }

  await page.screenshot({ path: '/home/cahya/.gemini/antigravity/brain/e54bdc71-6cfa-4c98-8627-949ef3da6eea/playwright_web_scheduler_dashboard.png', fullPage: true });

  await browser.close();
  process.exit(content.includes('Scheduler') ? 0 : 1);
})();
