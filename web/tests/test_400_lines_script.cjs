const { chromium } = require('playwright');
const path = require('path');
const fs = require('fs');

async function test400LinesScript() {
  console.log('🚀 Starting Playwright Test for 400-line Python script against http://127.0.0.1:8080 ...');
  
  const artifactDir = '/home/cahya/.gemini/antigravity/brain/e54bdc71-6cfa-4c98-8627-949ef3da6eea';
  if (!fs.existsSync(artifactDir)) {
    fs.mkdirSync(artifactDir, { recursive: true });
  }

  // Generate 400 lines of Python code
  let pythonCode = '# Python script 400 lines test\nimport sys\nimport os\n\ndef process_data():\n';
  for (let i = 1; i <= 395; i++) {
    pythonCode += `    print(f"Line ${i}: processing batch {${i} * 2}")\n`;
  }
  pythonCode += '\nif __name__ == "__main__":\n    process_data()\n    print("Done")\n\nini script apa';

  console.log(`Generated Python script with ${pythonCode.split('\n').length} lines (${pythonCode.length} chars).`);

  const browser = await chromium.launch({
    headless: true,
    args: ['--no-sandbox', '--disable-setuid-sandbox']
  });

  const context = await browser.newContext({
    viewport: { width: 1280, height: 900 }
  });

  const page = await context.newPage();

  try {
    console.log('1. Navigating to http://127.0.0.1:8080 ...');
    await page.goto('http://127.0.0.1:8080', { waitUntil: 'networkidle', timeout: 30000 });

    console.log('2. Waiting for chat textarea selector ...');
    const textareaSelector = 'textarea, input[type="text"]';
    await page.waitForSelector(textareaSelector, { timeout: 15000 });

    console.log('3. Pasting 400-line Python script ...');
    await page.fill(textareaSelector, pythonCode);
    await page.waitForTimeout(500);

    console.log('4. Pressing Enter to send prompt ...');
    await page.keyboard.press('Enter');

    console.log('5. Waiting 10s for response streaming and completion ...');
    await page.waitForTimeout(10000);

    const shotPath = path.join(artifactDir, 'playwright_400_lines_test.png');
    await page.screenshot({ path: shotPath, fullPage: true });
    console.log(`📸 400-line script test screenshot saved to: ${shotPath}`);

    console.log('✅ 400-line Python script Playwright test completed successfully!');

  } catch (err) {
    console.error('❌ Error during 400-line script test:', err);
    const errPath = path.join(artifactDir, 'playwright_400_lines_error.png');
    await page.screenshot({ path: errPath, fullPage: true }).catch(() => {});
    process.exitCode = 1;
  } finally {
    await browser.close();
  }
}

test400LinesScript();
