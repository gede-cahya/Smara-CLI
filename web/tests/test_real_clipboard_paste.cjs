const { chromium } = require('playwright');
const path = require('path');
const fs = require('fs');

async function testClipboardPaste() {
  console.log('🚀 Starting Real Clipboard Image Paste Playwright Test against http://127.0.0.1:5173 ...');
  
  const artifactDir = '/home/cahya/.gemini/antigravity/brain/e54bdc71-6cfa-4c98-8627-949ef3da6eea';
  if (!fs.existsSync(artifactDir)) {
    fs.mkdirSync(artifactDir, { recursive: true });
  }

  const browser = await chromium.launch({
    headless: true,
    args: ['--no-sandbox', '--disable-setuid-sandbox']
  });

  const context = await browser.newContext({
    viewport: { width: 1280, height: 900 },
    permissions: ['clipboard-read', 'clipboard-write']
  });

  const page = await context.newPage();

  try {
    console.log('1. Navigating to http://127.0.0.1:5173 ...');
    await page.goto('http://127.0.0.1:5173', { waitUntil: 'networkidle', timeout: 30000 });

    console.log('2. Waiting for chat input selector ...');
    const textareaSelector = 'textarea, input[type="text"]';
    await page.waitForSelector(textareaSelector, { timeout: 15000 });

    const sampleImagePath = '/home/cahya/.smara/clip-images/clip-web-1785482507459612894.png';
    const imageBase64 = fs.readFileSync(sampleImagePath).toString('base64');

    console.log('3. Dispatching synthetic PasteEvent with clipboard image data ...');
    await page.evaluate(async ({ base64Data }) => {
      const byteCharacters = atob(base64Data);
      const byteNumbers = new Array(byteCharacters.length);
      for (let i = 0; i < byteCharacters.length; i++) {
        byteNumbers[i] = byteCharacters.charCodeAt(i);
      }
      const byteArray = new Uint8Array(byteNumbers);
      const blob = new Blob([byteArray], { type: 'image/png' });
      const file = new File([blob], 'clipboard_pasted_image.png', { type: 'image/png' });

      const dataTransfer = new DataTransfer();
      dataTransfer.items.add(file);

      const textarea = document.querySelector('textarea, input[type="text"]');
      if (textarea) {
        textarea.focus();
        const pasteEvent = new ClipboardEvent('paste', {
          clipboardData: dataTransfer,
          bubbles: true,
          cancelable: true
        });
        textarea.dispatchEvent(pasteEvent);
      }
    }, { base64Data: imageBase64 });

    await page.waitForTimeout(1000);

    console.log('4. Typing prompt text "ini gambar apa dan jelaskan" ...');
    await page.fill(textareaSelector, 'ini gambar apa dan jelaskan');
    await page.waitForTimeout(500);

    console.log('5. Pressing Enter to send prompt with clipboard image ...');
    await page.keyboard.press('Enter');

    console.log('6. Waiting 10s for response streaming and completion ...');
    await page.waitForTimeout(10000);

    const shotPath = path.join(artifactDir, 'playwright_clipboard_paste_test.png');
    await page.screenshot({ path: shotPath, fullPage: true });
    console.log(`📸 Clipboard paste test screenshot saved to: ${shotPath}`);

    console.log('✅ Real Clipboard Image Paste Playwright Test completed successfully!');

  } catch (err) {
    console.error('❌ Error during clipboard paste test:', err);
    const errPath = path.join(artifactDir, 'playwright_clipboard_paste_error.png');
    await page.screenshot({ path: errPath, fullPage: true }).catch(() => {});
    process.exitCode = 1;
  } finally {
    await browser.close();
  }
}

testClipboardPaste();
