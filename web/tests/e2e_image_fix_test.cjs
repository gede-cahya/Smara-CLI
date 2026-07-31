const { chromium } = require('playwright');
const path = require('path');
const fs = require('fs');

async function testImagePromptNoStuck() {
  console.log('🚀 Testing image prompt does NOT get stuck after MCP context fix...');

  const artifactDir = '/home/cahya/.gemini/antigravity/brain/e54bdc71-6cfa-4c98-8627-949ef3da6eea';

  // Create a small test image (1x1 red pixel PNG)
  const testImgPath = '/tmp/test_red_pixel.png';
  if (!fs.existsSync(testImgPath)) {
    // Minimal valid PNG (1x1 red pixel)
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
    console.log('1. Navigating to http://127.0.0.1:8080 ...');
    await page.goto('http://127.0.0.1:8080', { waitUntil: 'networkidle', timeout: 15000 });
    
    console.log('2. Looking for chat textarea...');
    const textarea = await page.waitForSelector('textarea', { timeout: 10000 });
    
    // First test: long code prompt (should not stuck)
    console.log('3. Testing 400-line code prompt...');
    let codeLines = [];
    for (let i = 0; i < 400; i++) {
      codeLines.push(`    print("Line ${i}: processing data batch ${i * 100}")`);
    }
    const codePrompt = `ini script apa jelaskan:\n\ndef main():\n${codeLines.join('\n')}\n\nif __name__ == "__main__":\n    main()`;
    
    await textarea.fill(codePrompt);
    console.log(`   Filled ${codePrompt.length} chars of code prompt`);
    
    // Submit
    const sendBtn = await page.$('button[type="submit"], button:has(svg)');
    if (sendBtn) {
      await sendBtn.click();
      console.log('   Sent code prompt via button click');
    } else {
      await textarea.press('Enter');
      console.log('   Sent code prompt via Enter');
    }
    
    // Wait for response — should NOT be stuck
    const startCode = Date.now();
    let codeCompleted = false;
    
    for (let i = 0; i < 60; i++) { // max 60s
      await page.waitForTimeout(1000);
      const elapsed = ((Date.now() - startCode) / 1000).toFixed(1);
      
      // Check for response text or status change
      const statusEls = await page.$$eval('[class*="status"], [class*="phase"], [class*="thinking"]', els => 
        els.map(e => e.textContent).join(' | ')
      );
      const msgCount = await page.$$eval('[class*="message"], [class*="chat-bubble"], [class*="markdown"]', els => els.length);
      
      console.log(`   [${elapsed}s] messages: ${msgCount}, status: ${statusEls.slice(0, 100)}`);
      
      // Check if still stuck in "running" state
      const isRunning = statusEls.toLowerCase().includes('running') || statusEls.toLowerCase().includes('berjalan');
      const isCompleted = statusEls.toLowerCase().includes('completed') || statusEls.toLowerCase().includes('idle') || statusEls.toLowerCase().includes('selesai');
      
      if (isCompleted || (msgCount > 1 && !isRunning)) {
        console.log(`   ✅ Code prompt completed in ${elapsed}s!`);
        codeCompleted = true;
        break;
      }
      
      if (i > 30 && isRunning) {
        console.log(`   ❌ STUCK at "running" for ${elapsed}s — FIX FAILED`);
        break;
      }
    }
    
    // Take screenshot
    await page.screenshot({ path: path.join(artifactDir, 'playwright_code_test_fixed.png'), fullPage: true });
    console.log(`   Screenshot saved: playwright_code_test_fixed.png`);
    
    // Wait a bit then test image prompt
    console.log('\n4. Testing image prompt with clipboard simulation...');
    
    // Navigate fresh
    await page.goto('http://127.0.0.1:8080', { waitUntil: 'networkidle', timeout: 15000 });
    const textarea2 = await page.waitForSelector('textarea', { timeout: 10000 });
    
    // Type image prompt with [image:] tag
    const imagePrompt = `[image:${testImgPath}] ini gambar apa dan jelaskan`;
    await textarea2.fill(imagePrompt);
    console.log(`   Filled image prompt: ${imagePrompt}`);
    
    const sendBtn2 = await page.$('button[type="submit"], button:has(svg)');
    if (sendBtn2) {
      await sendBtn2.click();
      console.log('   Sent image prompt via button click');
    } else {
      await textarea2.press('Enter');
    }
    
    // Wait for response
    const startImg = Date.now();
    let imageCompleted = false;
    
    for (let i = 0; i < 60; i++) {
      await page.waitForTimeout(1000);
      const elapsed = ((Date.now() - startImg) / 1000).toFixed(1);
      
      const statusEls = await page.$$eval('[class*="status"], [class*="phase"], [class*="thinking"]', els => 
        els.map(e => e.textContent).join(' | ')
      );
      const msgCount = await page.$$eval('[class*="message"], [class*="chat-bubble"], [class*="markdown"]', els => els.length);
      
      console.log(`   [${elapsed}s] messages: ${msgCount}, status: ${statusEls.slice(0, 100)}`);
      
      const isRunning = statusEls.toLowerCase().includes('running') || statusEls.toLowerCase().includes('berjalan');
      const isCompleted = statusEls.toLowerCase().includes('completed') || statusEls.toLowerCase().includes('idle') || statusEls.toLowerCase().includes('selesai');
      
      if (isCompleted || (msgCount > 1 && !isRunning)) {
        console.log(`   ✅ Image prompt completed in ${elapsed}s!`);
        imageCompleted = true;
        break;
      }
      
      if (i > 30 && isRunning) {
        console.log(`   ❌ STUCK at "running" for ${elapsed}s — IMAGE FIX FAILED`);
        break;
      }
    }
    
    await page.screenshot({ path: path.join(artifactDir, 'playwright_image_test_fixed.png'), fullPage: true });
    console.log(`   Screenshot saved: playwright_image_test_fixed.png`);
    
    // Summary
    console.log('\n=== RESULTS ===');
    console.log(`Code prompt (400 lines): ${codeCompleted ? '✅ PASS' : '❌ FAIL'}`);
    console.log(`Image prompt: ${imageCompleted ? '✅ PASS' : '❌ FAIL'}`);
    
  } catch (err) {
    console.error('Test error:', err.message);
    await page.screenshot({ path: path.join(artifactDir, 'playwright_error.png'), fullPage: true });
  } finally {
    await browser.close();
    console.log('\nBrowser closed.');
  }
}

testImagePromptNoStuck().catch(console.error);
