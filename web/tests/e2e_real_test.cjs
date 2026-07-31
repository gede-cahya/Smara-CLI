const { chromium } = require('playwright');
const path = require('path');
const fs = require('fs');

async function runRealE2ETest() {
  console.log('🚀 Starting Playwright E2E Real Test against http://127.0.0.1:8080 ...');
  
  const artifactDir = '/home/cahya/.gemini/antigravity/brain/e54bdc71-6cfa-4c98-8627-949ef3da6eea';
  if (!fs.existsSync(artifactDir)) {
    fs.mkdirSync(artifactDir, { recursive: true });
  }

  const browser = await chromium.launch({
    headless: true,
    args: ['--no-sandbox', '--disable-setuid-sandbox']
  });

  const context = await browser.newContext({
    viewport: { width: 1280, height: 800 }
  });

  const page = await context.newPage();

  try {
    console.log('1. Navigating to http://127.0.0.1:8080 ...');
    await page.goto('http://127.0.0.1:8080', { waitUntil: 'networkidle', timeout: 30000 });

    // Wait for prompt textarea or input
    console.log('2. Waiting for chat input field ...');
    const textareaSelector = 'textarea, input[type="text"]';
    await page.waitForSelector(textareaSelector, { timeout: 15000 });

    // Test 1: Paste Code Explanation Prompt
    const codePrompt = `if __name__ == "__main__":\n    main() ini script apa`;
    console.log('3. Typing code prompt: "if __name__ == \\"__main__\\":\\n    main() ini script apa" ...');
    await page.fill(textareaSelector, codePrompt);
    await page.waitForTimeout(500);

    // Click submit button
    const submitBtn = page.locator('button[type="submit"], button:has(svg), button.bg-emerald-600, button.bg-primary').last();
    console.log('4. Clicking submit button ...');
    await page.keyboard.press('Enter');
    await page.waitForTimeout(1000);

    console.log('5. Waiting for prompt response stream / completion ...');
    // Wait up to 30s for assistant response message or status update
    await page.waitForTimeout(5000);

    const shot1Path = path.join(artifactDir, 'playwright_code_test.png');
    await page.screenshot({ path: shot1Path, fullPage: true });
    console.log(`📸 Code prompt test screenshot saved to: ${shot1Path}`);

    // Test 2: Send Image Explanation Prompt
    console.log('6. Testing image explanation prompt: "ini gambar apa" ...');
    await page.fill(textareaSelector, 'ini gambar apa');
    await page.waitForTimeout(500);
    await page.keyboard.press('Enter');

    console.log('7. Waiting for response ...');
    await page.waitForTimeout(5000);

    const shot2Path = path.join(artifactDir, 'playwright_image_test.png');
    await page.screenshot({ path: shot2Path, fullPage: true });
    console.log(`📸 Image prompt test screenshot saved to: ${shot2Path}`);

    console.log('✅ Real Playwright E2E test finished successfully!');

  } catch (err) {
    console.error('❌ Playwright E2E test error:', err);
    const errPath = path.join(artifactDir, 'playwright_error.png');
    await page.screenshot({ path: errPath, fullPage: true }).catch(() => {});
    process.exitCode = 1;
  } finally {
    await browser.close();
  }
}

runRealE2ETest();
