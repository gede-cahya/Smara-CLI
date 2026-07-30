import urllib.request
import urllib.parse
import json
import os
import time

WEBHOOK_URL = os.environ.get("DISCORD_WEBHOOK_URL", "https://discord.com/api/webhooks/1532388648706904195/cxmPEmPph7sWSZlkjz1MzYgJrD-5QsF0e8brQoQ2X1BLhX_LvMQ7p-6n8Mi0VuzhGQjk")
NODE_LOC_URL = "https://www.nodeloc.com/search.json?q=AI"

def translate_to_id(text):
    try:
        url = f"https://translate.googleapis.com/translate_a/single?client=gtx&sl=auto&tl=id&dt=t&q={urllib.parse.quote(text)}"
        req = urllib.request.Request(url, headers={"User-Agent": "Mozilla/5.0"})
        with urllib.request.urlopen(req, timeout=5) as resp:
            res = json.loads(resp.read().decode("utf-8"))
            translated = "".join([segment[0] for segment in res[0] if segment and segment[0]])
            return translated if translated else text
    except Exception as e:
        print(f"Gagal translate '{text}': {e}")
        return text

def send_topic_to_discord(topic):
    title_orig = topic.get("title", "No Title")
    topic_id = topic.get("id")
    url = f"https://www.nodeloc.com/t/topic/{topic_id}"
    
    title_id = translate_to_id(title_orig)
    
    thread_name = title_id[:100]  # Discord thread name max 100 chars
    
    content = f"**{title_id}**\n\n📌 **Judul Asli:** {title_orig}\n🔗 **Link:** {url}"

    payload = {
        "content": content,
        "thread_name": f"🤖 {thread_name}"
    }

    req_webhook = urllib.request.Request(
        WEBHOOK_URL,
        data=json.dumps(payload).encode("utf-8"),
        headers={"Content-Type": "application/json", "User-Agent": "Mozilla/5.0"}
    )
    try:
        with urllib.request.urlopen(req_webhook) as resp:
            print(f"Berhasil dikirim: {title_id} (Status {resp.status})")
    except Exception as e:
        print(f"Gagal kirim {title_id}: {e}")

def fetch_and_notify():
    req = urllib.request.Request(NODE_LOC_URL, headers={"User-Agent": "Mozilla/5.0"})
    with urllib.request.urlopen(req) as resp:
        data = json.loads(resp.read().decode("utf-8"))
    
    topics = data.get("topics", [])
    if not topics:
        print("Tidak ada topik ditemukan.")
        return

    # Kirim 5 topik terbaru satu per satu sebagai post/thread terpisah
    for topic in topics[:5]:
        send_topic_to_discord(topic)
        time.sleep(1) # Beri jeda kecil agar tidak kena rate limit Discord

if __name__ == "__main__":
    fetch_and_notify()
