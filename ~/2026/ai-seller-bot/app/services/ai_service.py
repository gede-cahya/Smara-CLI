import httpx
import json
from app.config import Config


class AIService:
    """Proxy requests to 9Router AI Gateway (OpenAI-compatible)."""

    def __init__(self):
        self.base_url = Config.NINEROUTER_URL
        self.api_key = Config.NINEROUTER_API_KEY

    def _parse_response(self, raw: bytes) -> dict:
        """Parse response from 9Router - handles JSON, multiple JSONs, and SSE formats."""
        text = raw.decode("utf-8", errors="replace").strip()

        # 1. Try direct JSON parse
        try:
            return json.loads(text)
        except json.JSONDecodeError:
            pass

        # 2. Use JSONDecoder to find first complete JSON object (handles extra data)
        decoder = json.JSONDecoder()
        text_stripped = text.lstrip()
        if text_stripped.startswith("{"):
            try:
                obj, _ = decoder.raw_decode(text_stripped)
                return obj
            except json.JSONDecodeError:
                pass

        # 3. Parse SSE chunks
        chunks = []
        for line in text.split("\n"):
            line = line.strip()
            if line.startswith("data: ") and "[DONE]" not in line:
                try:
                    chunks.append(json.loads(line[6:]))
                except json.JSONDecodeError:
                    continue

        if chunks:
            return self._merge_chunks(chunks)

        raise ValueError(f"Cannot parse 9Router response: {text[:300]}")

    def _merge_chunks(self, chunks: list) -> dict:
        """Merge SSE chunks into a single response."""
        content_parts = []
        model = None
        usage = None
        finish_reason = None

        for chunk in chunks:
            if "model" in chunk:
                model = chunk["model"]
            if "usage" in chunk:
                usage = chunk["usage"]
            for choice in chunk.get("choices", []):
                delta = choice.get("delta", {})
                if "content" in delta and delta["content"]:
                    content_parts.append(delta["content"])
                if choice.get("finish_reason"):
                    finish_reason = choice["finish_reason"]

        return {
            "id": chunks[-1].get("id", "chatcmpl-unknown"),
            "object": "chat.completion",
            "created": chunks[-1].get("created", 0),
            "model": model or "unknown",
            "choices": [
                {
                    "index": 0,
                    "message": {"role": "assistant", "content": "".join(content_parts)},
                    "finish_reason": finish_reason or "stop",
                }
            ],
            "usage": usage or {"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
        }

    async def chat(self, model: str, messages: list, temperature: float = 0.7, max_tokens: int = 2048) -> dict:
        """Send chat completion request to 9Router."""
        url = f"{self.base_url}/v1/chat/completions"
        headers = {
            "Authorization": f"Bearer {self.api_key}",
            "Content-Type": "application/json",
        }
        payload = {
            "model": model,
            "messages": messages,
            "temperature": temperature,
            "max_tokens": max_tokens,
            "stream": False,
        }

        async with httpx.AsyncClient(timeout=120.0) as client:
            resp = await client.post(url, headers=headers, json=payload)
            resp.raise_for_status()
            return self._parse_response(resp.content)

    async def get_models(self) -> list:
        """List available models from 9Router."""
        url = f"{self.base_url}/v1/models"
        headers = {"Authorization": f"Bearer {self.api_key}"}
        async with httpx.AsyncClient(timeout=30.0) as client:
            resp = await client.get(url, headers=headers)
            resp.raise_for_status()
            data = resp.json()
            return [m["id"] for m in data.get("data", [])]


ai_service = AIService()
