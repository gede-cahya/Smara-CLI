import urllib.request
import urllib.parse
import json
import os
import time

WEBHOOK_URL = os.environ.get("DISCORD_WEBHOOK_URL", "https://discord.com/api/webhooks/1532388648706904195/cxmPEmPph7sWSZlkjz1MzYgJrD-5QsF0e8brQoQ2X1BLhX_LvMQ7p-6n8Mi0VuzhGQjk")
NODE_LOC_URL = "https://www.nodeloc.com/search.json?q=AI"
STATE_FILE = "posted_topics.json"

def load_posted_ids():
    if os.path.exists(STATE_FILE):
        try:
            with open(STATE_FILE, "r", encoding="utf-8") as f:
                return set(json.load(f))
        except Exception as e:
            print(f"Error loading state file: {e}")
    return set()

def save_posted_ids(posted_set):
    try:
        with open(STATE_FILE, "w", encoding="utf-8") as f:
            json.dump(list(posted_set), f, indent=2)
    except Exception as e:
        print(f"Error saving state file: {e}")

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
    thread_name = title_id[:100]
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
            return True
    except Exception as e:
        print(f"Gagal kirim {title_id}: {e}")
        return False

def fetch_and_notify():
    posted = load_posted_ids()
    posted_str = {str(x) for x in posted}
    is_initial = not posted

    req = urllib.request.Request(NODE_LOC_URL, headers={"User-Agent": "Mozilla/5.0"})
    with urllib.request.urlopen(req) as resp:
        data = json.loads(resp.read().decode("utf-8"))
    
    topics = data.get("topics", [])
    if not topics:
        print("Tidak ada topik ditemukan.")
        return

    seen = set()
    new_topics = []
    for t in topics:
        tid, title = str(t.get("id")), t.get("title", "")
        if tid not in posted_str and title not in posted_str and tid not in seen:
            seen.add(tid)
            new_topics.append(t)

    if not new_topics:
        print("Tidak ada topik NodeLoc baru.")
        return

    items_to_send = new_topics[:5] if is_initial else new_topics

    for topic in items_to_send:
        if send_topic_to_discord(topic):
            posted.update({str(topic.get("id")), topic.get("title")})
            time.sleep(1)

    if is_initial:
        for t in topics:
            posted.update({str(t.get("id")), t.get("title")})

    save_posted_ids(posted)

if __name__ == "__main__":
    fetch_and_notify()
