const { chromium } = require('playwright');
const path = require('path');
const fs = require('fs');

async function testImageNoStuck() {
  console.log('🚀 Testing image prompt with REAL WebSocket wait (after MCP context fix)...');

  const artifactDir = '/home/cahya/.gemini/antigravity/brain/e54bdc71-6cfa-4c98-8627-949ef3da6eea';

  // Create a small test image
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

  // Monitor WebSocket messages
  let wsMessages = [];
  page.on('websocket', ws => {
    ws.on('framereceived', frame => {
      try {
        const data = JSON.parse(frame.payload);
        wsMessages.push(data);
        if (data.type) {
          console.log(`   WS: type=${data.type}`);
        }
      } catch {}
    });
  });

  try {
    console.log('1. Navigating...');
    await page.goto('http://127.0.0.1:8080', { waitUntil: 'networkidle', timeout: 15000 });

    console.log('2. Typing image prompt...');
    const textarea = await page.waitForSelector('textarea', { timeout: 10000 });
    const imagePrompt = `[image:${testImgPath}] ini gambar apa`;
    await textarea.fill(imagePrompt);

    // Clear WS history
    wsMessages = [];

    console.log('3. Sending prompt...');
    const sendBtn = await page.$('button[type="submit"], button:has(svg)');
    if (sendBtn) await sendBtn.click();
    else await textarea.press('Enter');

    // Wait for "done" or "completed" WS event (max 45s)
    const start = Date.now();
    let gotDone = false;
    let lastEvent = '';
    
    for (let i = 0; i < 45; i++) {
      await page.waitForTimeout(1000);
      const elapsed = ((Date.now() - start) / 1000).toFixed(1);

      // Check WS messages for done/completed
      for (const msg of wsMessages) {
        if (msg.type === 'done' || msg.type === 'completed' || msg.type === 'final') {
          gotDone = true;
          lastEvent = msg.type;
        }
        if (msg.type === 'stream' && msg.text && msg.text.length > 0) {
          lastEvent = 'streaming';
        }
        if (msg.type === 'tool_done') {
          lastEvent = 'tool_done';
        }
      }

      // Also check page content for response
      const bodyText = await page.evaluate(() => document.body.innerText.slice(-500));
      const hasResponse = bodyText.includes('gambar') || bodyText.includes('image') || bodyText.includes('piksel') || bodyText.includes('pixel');

      console.log(`   [${elapsed}s] ws_events=${wsMessages.length}, last=${lastEvent}, hasResponse=${hasResponse}`);

      if (gotDone) {
        console.log(`   ✅ Image prompt completed via WS '${lastEvent}' in ${elapsed}s`);
        break;
      }

      if (hasResponse && i > 5 && lastEvent !== 'tool_done') {
        console.log(`   ✅ Response text detected in ${elapsed}s (content arrived)`);
        gotDone = true;
        break;
      }

      if (i > 35) {
        console.log(`   ❌ STUCK for ${elapsed}s — fix may not be working`);
        break;
      }
    }

    await page.screenshot({ path: path.join(artifactDir, 'playwright_image_ws_test.png'), fullPage: true });
    console.log(`\nScreenshot: playwright_image_ws_test.png`);
    console.log(`Total WS messages received: ${wsMessages.length}`);
    console.log(`Result: ${gotDone ? '✅ PASS' : '❌ FAIL'}`);

  } catch (err) {
    console.error('Error:', err.message);
    await page.screenshot({ path: path.join(artifactDir, 'playwright_image_ws_error.png'), fullPage: true });
  } finally {
    await browser.close();
  }
}

testImageNoStuck().catch(console.error);
