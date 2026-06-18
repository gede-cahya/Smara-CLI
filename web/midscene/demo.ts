import { chromium } from 'playwright';
import { PlaywrightAgent } from '@midscene/web/playwright';
import 'dotenv/config';

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

async function main() {
  const browser = await chromium.launch({ headless: false });
  const page = await browser.newPage();

  await page.goto('https://www.ebay.com');
  await sleep(3000);

  const agent = new PlaywrightAgent(page);

  await agent.aiAct('type "Headphones" in the search box, then hit Enter');
  await agent.aiWaitFor('there is at least one headphone product in the list');

  const items = await agent.aiQuery(
    '{ title: string, price: number }[], the headphone products in the list',
  );
  console.log('headphones in stock:', items);

  await agent.aiAssert('There is a category filter on the left side');

  await browser.close();
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
