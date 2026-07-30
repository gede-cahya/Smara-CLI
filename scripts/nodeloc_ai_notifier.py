import urllib.request
import json
import os

WEBHOOK_URL = os.environ.get("DISCORD_WEBHOOK_URL", "https://discord.com/api/webhooks/1532388648706904195/cxmPEmPph7sWSZlkjz1MzYgJrD-5QsF0e8brQoQ2X1BLhX_LvMQ7p-6n8Mi0VuzhGQjk")
NODE_LOC_URL = "https://www.nodeloc.com/search.json?q=AI"

def fetch_nodeloc_ai():
    req = urllib.request.Request(NODE_LOC_URL, headers={"User-Agent": "Mozilla/5.0"})
    with urllib.request.urlopen(req) as resp:
        data = json.loads(resp.read().decode("utf-8"))
    
    topics = data.get("topics", [])
    if not topics:
        print("Tidak ada topik ditemukan.")
        return

    content_lines = ["**🤖 Topik AI Terbaru dari Forum NodeLoc:**\n"]
    for t in topics[:5]:
        title = t.get("title", "No Title")
        topic_id = t.get("id")
        url = f"https://www.nodeloc.com/t/topic/{topic_id}"
        content_lines.append(f"- [{title}]({url})")

    payload = {
        "content": "\n".join(content_lines),
        "thread_name": "🔥 Topik AI NodeLoc Terbaru"
    }

    req_webhook = urllib.request.Request(
        WEBHOOK_URL,
        data=json.dumps(payload).encode("utf-8"),
        headers={"Content-Type": "application/json", "User-Agent": "Mozilla/5.0"}
    )
    with urllib.request.urlopen(req_webhook) as resp:
        print("Status:", resp.status)
    print("Berhasil dikirim ke Discord Forum Channel!")

if __name__ == "__main__":
    fetch_nodeloc_ai()
