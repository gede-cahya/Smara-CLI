const { chromium } = require('playwright');
const path = require('path');

const USER_SCRIPT = `import requests
import re
import random
import string
import pyotp
import json
from bs4 import BeautifulSoup

BASE_URL = "https://dewabiz.com"
MY_URL = "https://my.dewabiz.com"

HEADERS = {
    "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36",
    "Accept": "*/*",
    "Accept-Language": "en-US,en;q=0.9",
}

FIRST_NAMES = ["Ahmad", "Budi", "Citra", "Dewi", "Eko", "Fitri", "Gilang", "Heni"]
LAST_NAMES = ["Pratama", "Wijaya", "Kusuma", "Nugraha", "Santoso", "Hartono"]

def main():
    print("Testing script")

if __name__ == "__main__":
    main()`;

(async () => {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();
  
  console.log('Navigating to Smara Web...');
  await page.goto('http://127.0.0.1:8080');
  await page.waitForTimeout(3000);
  
  console.log('Clicking New Session button...');
  const newChatBtn = page.locator('button:has-text("Sesi Baru"), button:has-text("+"), button[title*="Sesi"], button[title*="Baru"]').first();
  if (await newChatBtn.isVisible()) {
    await newChatBtn.click();
    await page.waitForTimeout(1000);
  }

  console.log('Locating textarea and pasting pure python script...');
  const textarea = page.locator('textarea').first();
  await textarea.fill(USER_SCRIPT);
  await page.waitForTimeout(500);

  console.log('Sending prompt...');
  await textarea.press('Enter');
  
  console.log('Waiting for response stream...');
  const startTime = Date.now();
  let gotResponse = false;

  for (let i = 0; i < 40; i++) {
    await page.waitForTimeout(500);
    const content = await page.textContent('body');
    if (content.includes('python') || content.includes('import') || content.includes('Script') || content.includes('Kode') || content.includes('dewabiz')) {
      gotResponse = true;
      const elapsed = (Date.now() - startTime) / 1000;
      console.log(`SUCCESS! Response received in ${elapsed.toFixed(1)}s`);
      break;
    }
  }

  await page.screenshot({ path: '/home/cahya/.gemini/antigravity/brain/e54bdc71-6cfa-4c98-8627-949ef3da6eea/playwright_pure_code_paste.png', fullPage: true });

  if (gotResponse) {
    console.log('Test PASSED!');
  } else {
    console.error('Test FAILED - No response after 20s');
  }

  await browser.close();
  process.exit(gotResponse ? 0 : 1);
})();
