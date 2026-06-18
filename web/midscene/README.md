# MidsceneJS for Smara Web

MidsceneJS is integrated for vision-driven UI automation on top of Playwright.

## Setup

```bash
cd web
cp .env.midscene.example .env
# edit .env and set MIDSCENE_MODEL_API_KEY
npm install
```

Default model configuration follows the Midscene quick start:

```bash
MIDSCENE_MODEL_BASE_URL="https://openrouter.ai/api/v1"
MIDSCENE_MODEL_API_KEY="your-openrouter-api-key"
MIDSCENE_MODEL_NAME="qwen/qwen3.7-plus"
MIDSCENE_MODEL_FAMILY="qwen3"
```

## Run official-style demo

```bash
cd web
npm run midscene:demo
```

This opens eBay, searches for headphones, queries product data, and asserts the page with natural language.

## Run against local Smara web with direct PlaywrightAgent

Terminal 1:

```bash
cd web
npm run dev
```

Terminal 2:

```bash
cd web
npm run midscene:web
```

Optional overrides:

```bash
SMARA_WEB_URL="http://localhost:5173" \
MIDSCENE_PROMPT="find the chat input and describe how to start a new task" \
HEADLESS="false" \
npm run midscene:web
```

## Run integrated Playwright test runner

This follows Midscene's Playwright integration via `PlaywrightAiFixture` from `@midscene/web/playwright` and the Midscene Playwright reporter.

Terminal 1:

```bash
cd web
npm run dev
```

Terminal 2:

```bash
cd web
npm run midscene:playwright
```

Interactive Playwright UI mode:

```bash
cd web
npm run midscene:playwright:ui
```

Files:

```text
playwright.config.ts
midscene/tests/smara-web.spec.ts
```

## Report

After a successful run, Midscene prints a report path like:

```text
Midscene - report file updated: ./midscene_run/report/some_id.html
```

Open the HTML report to inspect every AI action/query/assertion step. Playwright HTML reports are written to `web/playwright-report/`.
