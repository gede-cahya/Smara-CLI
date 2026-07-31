const { chromium } = require('playwright');
const path = require('path');

async function testContinuationAnd400Lines() {
  console.log('🚀 Starting Playwright E2E test: 400-line script prompt + "lanjutkan" continuation...');
  const artifactDir = '/home/cahya/.gemini/antigravity/brain/e54bdc71-6cfa-4c98-8627-949ef3da6eea';

  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();

  try {
    console.log('1. Navigating to http://127.0.0.1:8080 ...');
    await page.goto('http://127.0.0.1:8080', { waitUntil: 'networkidle', timeout: 15000 });

    const textarea = await page.waitForSelector('textarea', { timeout: 10000 });

    // Generate 406-line Python script
    console.log('2. Preparing 406-line Python script...');
    let lines = [];
    for (let i = 1; i <= 400; i++) {
      lines.push(`    # Line ${i}: processing batch data for item ${i * 10}\n    item_${i} = {"id": ${i}, "val": "${i * 42}"}`);
    }
    const longScript = `def process_all_batches():\n${lines.join('\n')}\n    return True\n\nif __name__ == "__main__":\n    process_all_batches()\nanalisis script ini`;

    console.log(`   Script size: ${longScript.length} characters, ${longScript.split('\n').length} lines`);

    await textarea.fill(longScript);
    console.log('3. Sending 406-line script prompt...');

    const sendBtn = await page.$('button[type="submit"], button:has(svg)');
    if (sendBtn) await sendBtn.click();
    else await textarea.press('Enter');

    // Wait for response
    const start1 = Date.now();
    let step1Done = false;
    for (let i = 0; i < 40; i++) {
      await page.waitForTimeout(1000);
      const elapsed = ((Date.now() - start1) / 1000).toFixed(1);

      const isRunning = await page.evaluate(() => {
        const text = document.body.innerText.toLowerCase();
        return text.includes('thinking') || text.includes('analyzing') || text.includes('composing') || text.includes('generating');
      });

      const hasContent = await page.evaluate(() => {
        const text = document.body.innerText;
        return text.includes('def') || text.includes('process_all_batches') || text.includes('script') || text.includes('batch');
      });

      console.log(`   [${elapsed}s] running=${isRunning}, hasContent=${hasContent}`);

      if (hasContent && !isRunning && i > 5) {
        console.log(`   ✅ 406-line script analysis completed in ${elapsed}s!`);
        step1Done = true;
        break;
      }
    }

    await page.screenshot({ path: path.join(artifactDir, 'playwright_400lines_continuation_step1.png'), fullPage: true });

    // Step 2: Send "lanjutkan"
    console.log('\n4. Sending continuation prompt: "lanjutkan"...');
    const textarea2 = await page.waitForSelector('textarea', { timeout: 10000 });
    await textarea2.fill('lanjutkan');

    const sendBtn2 = await page.$('button[type="submit"], button:has(svg)');
    if (sendBtn2) await sendBtn2.click();
    else await textarea2.press('Enter');

    const start2 = Date.now();
    let step2Done = false;

    for (let i = 0; i < 40; i++) {
      await page.waitForTimeout(1000);
      const elapsed = ((Date.now() - start2) / 1000).toFixed(1);

      const isRunning = await page.evaluate(() => {
        const text = document.body.innerText.toLowerCase();
        return text.includes('thinking') || text.includes('analyzing') || text.includes('composing') || text.includes('generating');
      });

      const bodyText = await page.evaluate(() => document.body.innerText);
      const hasLanjutkanResponse = bodyText.includes('lanjutkan') || bodyText.length > 500;

      console.log(`   [${elapsed}s] continuation running=${isRunning}, len=${bodyText.length}`);

      if (!isRunning && i > 3) {
        console.log(`   ✅ Continuation ("lanjutkan") completed cleanly in ${elapsed}s!`);
        step2Done = true;
        break;
      }
    }

    await page.screenshot({ path: path.join(artifactDir, 'playwright_400lines_continuation_step2.png'), fullPage: true });

    console.log('\n=== RESULTS ===');
    console.log(`406-line script analysis: ${step1Done ? '✅ PASS' : '❌ FAIL'}`);
    console.log(`Continuation ("lanjutkan"): ${step2Done ? '✅ PASS' : '❌ FAIL'}`);

  } catch (err) {
    console.error('Error:', err.message);
  } finally {
    await browser.close();
  }
}

testContinuationAnd400Lines().catch(console.error);
