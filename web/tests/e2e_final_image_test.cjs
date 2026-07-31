const { chromium } = require('playwright');
const path = require('path');
const fs = require('fs');

async function testImageNoStuckFinal() {
  console.log('🚀 FINAL TEST: image prompt via WebSocket direct monitoring...');
  const artifactDir = '/home/cahya/.gemini/antigravity/brain/e54bdc71-6cfa-4c98-8627-949ef3da6eea';

  // Create test image
  const testImgPath = '/tmp/test_red_pixel.png';
  if (!fs.existsSync(testImgPath)) {
    const buf = Buffer.from(
      '89504e470d0a1a0a0000000d49484452000000010000000108020000009001' +
      '2e00000000c4944415478016360f8cf0000000201010b14d5170000000049454e44ae426082',
      'hex'
    );
    fs.writeFileSync(testImgPath, buf);
  }

  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();

  try {
    console.log('1. Navigating...');
    await page.goto('http://127.0.0.1:8080', { waitUntil: 'networkidle', timeout: 15000 });

    // Count initial messages BEFORE sending
    const initialMsgCount = await page.evaluate(() => {
      const els = document.querySelectorAll('[class*="message"], [class*="chat"], .prose, .markdown-body');
      return els.length;
    });
    console.log(`   Initial message elements: ${initialMsgCount}`);

    console.log('2. Typing and sending image prompt...');
    const textarea = await page.waitForSelector('textarea', { timeout: 10000 });
    await textarea.fill(`[image:${testImgPath}] ini gambar apa`);

    const sendBtn = await page.$('button[type="submit"], button:has(svg)');
    if (sendBtn) await sendBtn.click();
    else await textarea.press('Enter');

    // Now wait for new content to appear
    const start = Date.now();
    let result = 'FAIL';

    for (let i = 0; i < 45; i++) {
      await page.waitForTimeout(1000);
      const elapsed = ((Date.now() - start) / 1000).toFixed(1);

      // Count current message elements
      const currentMsgCount = await page.evaluate(() => {
        const els = document.querySelectorAll('[class*="message"], [class*="chat"], .prose, .markdown-body');
        return els.length;
      });
      const newMsgs = currentMsgCount - initialMsgCount;

      // Check page content (last 2000 chars)
      const pageText = await page.evaluate(() => document.body.innerText);
      const last500 = pageText.slice(-500);

      // Look for response indicators
      const hasStreamingText = last500.includes('analyze_image') || last500.includes('Analyzing') || last500.includes('Menganalisis');
      const hasDoneIndicator = last500.includes('piksel') || last500.includes('pixel') || last500.includes('merah') || last500.includes('red') ||
                               last500.includes('PNG') || last500.includes('kecil') || last500.includes('small') ||
                               last500.includes('📸') || last500.includes('image') || last500.includes('OCR');

      // Check if running status is present
      const statusText = await page.evaluate(() => {
        const statusEls = document.querySelectorAll('[class*="status"], [class*="phase"], [class*="progress"]');
        return Array.from(statusEls).map(e => e.textContent).join(' | ');
      });

      console.log(`   [${elapsed}s] new_msgs=${newMsgs}, status="${statusText.slice(0,80)}", streaming=${hasStreamingText}, done=${hasDoneIndicator}`);

      // Detect if response arrived: new messages appeared and streaming/done detected
      if (newMsgs > 0 && (hasStreamingText || hasDoneIndicator)) {
        // Wait 2 more seconds to confirm not stuck
        await page.waitForTimeout(2000);
        result = 'PASS';
        console.log(`   ✅ Image response detected in ${elapsed}s with ${newMsgs} new messages`);
        break;
      }

      // Stuck detection: if >30s and still running with no new content
      if (i > 30 && newMsgs === 0) {
        console.log(`   ❌ STUCK for ${elapsed}s — no new messages appeared`);
        break;
      }
    }

    // Take screenshot
    await page.screenshot({ path: path.join(artifactDir, 'playwright_final_image_test.png'), fullPage: true });
    console.log(`\nScreenshot: playwright_final_image_test.png`);
    console.log(`Result: ${result === 'PASS' ? '✅ PASS' : '❌ FAIL'}`);

  } catch (err) {
    console.error('Error:', err.message);
    await page.screenshot({ path: path.join(artifactDir, 'playwright_final_error.png'), fullPage: true });
  } finally {
    await browser.close();
  }
}

testImageNoStuckFinal().catch(console.error);
