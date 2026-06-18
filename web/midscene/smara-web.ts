import { chromium } from 'playwright';
import { PlaywrightAgent } from '@midscene/web/playwright';
import 'dotenv/config';

const targetUrl = process.env.SMARA_WEB_URL ?? 'http://localhost:5173';
const prompt = process.env.MIDSCENE_PROMPT ?? 'describe the main UI sections and important controls on this page';

async function main() {
  const browser = await chromium.launch({ headless: process.env.HEADLESS === 'true' });
  const page = await browser.newPage();

  await page.goto(targetUrl, { waitUntil: 'networkidle' });

  const agent = new PlaywrightAgent(page);
  const summary = await agent.aiQuery(`string, ${prompt}`);

  console.log('Midscene summary for', targetUrl);
  console.log(summary);

  await browser.close();
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
