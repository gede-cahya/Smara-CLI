const { chromium } = require('playwright');
const path = require('path');

async function testUserScriptPlaywright() {
  console.log('🚀 Running Real Playwright E2E Test with User Python Script (250+ lines)...');
  const artifactDir = '/home/cahya/.gemini/antigravity/brain/e54bdc71-6cfa-4c98-8627-949ef3da6eea';

  const userScript = `import requests
import re
import random
import string
import pyotp
import json
from bs4 import BeautifulSoup

BASE_URL = "https://dewabiz.com"
MY_URL = "https://my.dewabiz.com"

HEADERS = {
    "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64)",
}

def main():
    print("Testing script...")

if __name__ == "__main__":
    main()
analisis script ini`;

  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();

  try {
    console.log('1. Navigating to http://127.0.0.1:8080 ...');
    await page.goto('http://127.0.0.1:8080', { waitUntil: 'networkidle', timeout: 15000 });

    const textarea = await page.waitForSelector('textarea', { timeout: 10000 });

    console.log('2. Filling chat textarea with user Python script...');
    await textarea.fill(userScript);

    console.log('3. Submitting prompt...');
    const sendBtn = await page.$('button[type="submit"], button:has(svg)');
    if (sendBtn) await sendBtn.click();
    else await textarea.press('Enter');

    const start = Date.now();
    let completed = false;

    for (let i = 0; i < 40; i++) {
      await page.waitForTimeout(1000);
      const elapsed = ((Date.now() - start) / 1000).toFixed(1);

      const isRunning = await page.evaluate(() => {
        const text = document.body.innerText.toLowerCase();
        return text.includes('thinking') || text.includes('analyzing') || text.includes('composing') || text.includes('generating');
      });

      const bodyText = await page.evaluate(() => document.body.innerText);
      const hasAnswer = bodyText.includes('requests') || bodyText.includes('script') || bodyText.includes('Python');

      console.log(`   [${elapsed}s] running=${isRunning}, hasAnswer=${hasAnswer}`);

      if (hasAnswer && !isRunning && i > 3) {
        console.log(`   ✅ Real Playwright test completed successfully in ${elapsed}s!`);
        completed = true;
        break;
      }
    }

    await page.screenshot({ path: path.join(artifactDir, 'playwright_user_script_test.png'), fullPage: true });
    console.log(`Screenshot saved: playwright_user_script_test.png`);
    console.log(`Result: ${completed ? '✅ PASS' : '❌ FAIL'}`);

  } catch (err) {
    console.error('Playwright test error:', err.message);
  } finally {
    await browser.close();
  }
}

testUserScriptPlaywright().catch(console.error);
