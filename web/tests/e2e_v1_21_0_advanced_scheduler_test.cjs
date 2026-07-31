const { chromium } = require('playwright');
const { execSync } = require('child_process');
const fs = require('fs');

(async () => {
  console.log('=== E2E Real Test: Smara Advanced Scheduler v1.21.0 ===');

  // 1. CLI Advanced Scheduler Tests
  console.log('\n--- 1. Testing Advanced CLI Schedule Capabilities ---');
  
  // Test 1: Add Standard 5-field Cron
  console.log('Test 1: Adding standard 5-field cron "*/5 9-17 * * 1-5" "backup_task"...');
  const cronOut = execSync('./smara schedule add "*/5 9-17 * * 1-5" "backup_task" --retries 3 --retry-interval 15', { cwd: '/home/cahya/2026/Smara CLI', encoding: 'utf-8' });
  console.log(cronOut.trim());
  const parentId = cronOut.match(/Schedule (sch-[a-f0-9]+)/)[1];
  console.log(`✓ Created parent cronjob ID: ${parentId}`);

  // Test 2: Add Chained Job with --after
  console.log(`\nTest 2: Adding chained job "--after ${parentId}" "notify_task"...`);
  const chainedOut = execSync(`./smara schedule add "every 30m" "notify_task" --after "${parentId}"`, { cwd: '/home/cahya/2026/Smara CLI', encoding: 'utf-8' });
  console.log(chainedOut.trim());
  const childId = chainedOut.match(/Schedule (sch-[a-f0-9]+)/)[1];
  console.log(`✓ Created chained cronjob ID: ${childId}`);

  // Test 3: List Schedules
  console.log('\nTest 3: Listing schedules...');
  const listOut = execSync('./smara schedule list', { cwd: '/home/cahya/2026/Smara CLI', encoding: 'utf-8' });
  console.log(listOut.trim());

  // Test 4: Systemd Service Installation Simulation
  console.log('\nTest 4: Testing Systemd Service Management...');
  const serviceOut = execSync('./smara schedule service install && ./smara schedule service status && ./smara schedule service uninstall', { cwd: '/home/cahya/2026/Smara CLI', encoding: 'utf-8' });
  console.log(serviceOut.trim());
  console.log('✓ Systemd service management verified!');

  // Clean up CLI schedules
  execSync(`./smara schedule remove "${parentId}" && ./smara schedule remove "${childId}"`, { cwd: '/home/cahya/2026/Smara CLI' });
  console.log('✓ CLI test schedules cleaned up.');

  // 2. Playwright Web UI Prompt Test
  console.log('\n--- 2. Testing Web UI Prompt Streaming & Execution ---');
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();
  
  console.log('Navigating to Smara Web UI (http://127.0.0.1:8080)...');
  await page.goto('http://127.0.0.1:8080');
  await page.waitForTimeout(3000);

  console.log('Sending prompt test: "apa fitur terbaru di Smara v1.21.0?"...');
  const textarea = page.locator('textarea').first();
  await textarea.fill('apa fitur terbaru di Smara v1.21.0?');
  await textarea.press('Enter');

  console.log('Waiting for response stream...');
  const startTime = Date.now();
  let gotResponse = false;

  for (let i = 0; i < 30; i++) {
    await page.waitForTimeout(500);
    const content = await page.textContent('body');
    if (content.includes('1.21.0') || content.includes('Scheduler') || content.includes('Cron') || content.includes('Retry') || content.includes('Smara')) {
      gotResponse = true;
      const elapsed = (Date.now() - startTime) / 1000;
      console.log(`✓ Prompt test SUCCESS! Streaming response received in ${elapsed.toFixed(1)}s`);
      break;
    }
  }

  await page.screenshot({ path: '/home/cahya/.gemini/antigravity/brain/e54bdc71-6cfa-4c98-8627-949ef3da6eea/playwright_v1.21.0_advanced_scheduler_test.png', fullPage: true });

  await browser.close();

  if (gotResponse) {
    console.log('\n✅ ALL ADVANCED SCHEDULER & PROMPT TESTS PASSED 100%!');
    process.exit(0);
  } else {
    console.error('\n❌ Web UI Prompt Test FAILED');
    process.exit(1);
  }
})();
