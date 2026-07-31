const { chromium } = require('playwright');
const path = require('path');
const fs = require('fs');

async function testImageMakeDev() {
  console.log('🚀 Starting Playwright E2E Image Test against make dev URL http://127.0.0.1:5173 ...');
  
  const artifactDir = '/home/cahya/.gemini/antigravity/brain/e54bdc71-6cfa-4c98-8627-949ef3da6eea';
  if (!fs.existsSync(artifactDir)) {
    fs.mkdirSync(artifactDir, { recursive: true });
  }

  const browser = await chromium.launch({
    headless: true,
    args: ['--no-sandbox', '--disable-setuid-sandbox']
  });

  const context = await browser.newContext({
    viewport: { width: 1280, height: 900 }
  });

  const page = await context.newPage();

  try {
    console.log('1. Navigating to http://127.0.0.1:5173 ...');
    await page.goto('http://127.0.0.1:5173', { waitUntil: 'networkidle', timeout: 30000 });

    console.log('2. Waiting for chat input selector ...');
    const textareaSelector = 'textarea, input[type="text"]';
    await page.waitForSelector(textareaSelector, { timeout: 15000 });

    console.log('3. Sending image prompt "ini gambar apa" ...');
    await page.fill(textareaSelector, '[image:/home/cahya/.smara/clip-images/clip-web-1785482211066582281.png] ini gambar apa');
    await page.waitForTimeout(500);

    console.log('4. Pressing Enter ...');
    await page.keyboard.press('Enter');

    console.log('5. Waiting 10s for analyze_image execution & LLM completion ...');
    await page.waitForTimeout(10000);

    const shotPath = path.join(artifactDir, 'playwright_image_make_dev_test.png');
    await page.screenshot({ path: shotPath, fullPage: true });
    console.log(`📸 Image make dev test screenshot saved to: ${shotPath}`);

    console.log('✅ Image make dev Playwright E2E test finished successfully!');

  } catch (err) {
    console.error('❌ Error during image test:', err);
    const errPath = path.join(artifactDir, 'playwright_image_make_dev_error.png');
    await page.screenshot({ path: errPath, fullPage: true }).catch(() => {});
    process.exitCode = 1;
  } finally {
    await browser.close();
  }
}

testImageMakeDev();
