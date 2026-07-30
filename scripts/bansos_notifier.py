import urllib.request
import json
import os
import time

WEBHOOK_URL = os.environ.get("DISCORD_WEBHOOK_URL_BANSOS", "https://discord.com/api/webhooks/1532392433000775770/wXZOzkXMf3zxF3oSaoUwceAJJbF3dP0yNbLzV4iGVIOz7DXyUXpvRC8q2axv9yKjvaXO")
DATA_URL = "https://raw.githubusercontent.com/wauputr4/bansos/main/src/lib/data/bansos.json"
STATE_FILE = "posted_bansos.json"

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

def send_bansos_to_discord(item):
    item_id = item.get("id")
    title = item.get("title", "Info Bansos Developer")
    description = item.get("description", "")
    provider = item.get("provider", "-")
    promo_code = item.get("promoCode")
    cta_link = item.get("ctaLink", "https://bansos.dev/list/")
    benefits = item.get("benefits", [])
    tags = item.get("tags", [])
    status = item.get("status", "active")
    
    tags_str = " ".join([f"`#{t}`" for t in tags]) if tags else "`#bansos`"
    benefits_str = "\n".join([f"• {b}" for b in benefits]) if benefits else "-"
    code_str = f"`{promo_code}`" if promo_code else "Tidak ada / Otomatis"

    content = f"🎁 **{title}**\n\n" \
              f"**Provider:** {provider}\n" \
              f"**Deskripsi:** {description}\n\n" \
              f"✨ **Benefits:**\n{benefits_str}\n\n" \
              f"🎟️ **Kode Promo:** {code_str}\n" \
              f"🏷️ **Tags:** {tags_str}\n" \
              f"🔗 **Detail & Klaim:** {cta_link}"

    thread_name = title[:100]

    payload = {
        "content": content,
        "thread_name": f"🎁 {thread_name}"
    }

    req_webhook = urllib.request.Request(
        WEBHOOK_URL,
        data=json.dumps(payload).encode("utf-8"),
        headers={"Content-Type": "application/json", "User-Agent": "Mozilla/5.0"}
    )
    try:
        with urllib.request.urlopen(req_webhook) as resp:
            print(f"Berhasil dikirim: {title} (Status {resp.status})")
            return True
    except Exception as e:
        print(f"Gagal kirim {title}: {e}")
        return False

def fetch_and_notify():
    posted_ids = load_posted_ids()
    is_initial_run = len(posted_ids) == 0

    req = urllib.request.Request(DATA_URL, headers={"User-Agent": "Mozilla/5.0"})
    with urllib.request.urlopen(req) as resp:
        bansos_items = json.loads(resp.read().decode("utf-8"))
    
    if not bansos_items:
        print("Tidak ada bansos ditemukan.")
        return

    # Filter items that haven't been posted yet
    new_items = [b for b in bansos_items if b.get("id") not in posted_ids]

    if not new_items:
        print("Tidak ada bansos baru untuk dikirim.")
        return

    print(f"Ditemukan {len(new_items)} bansos baru.")

    # In initial run, limit to latest 5 to prevent webhook spam
    items_to_send = new_items[:5] if is_initial_run else new_items

    for item in items_to_send:
        if send_bansos_to_discord(item):
            posted_ids.add(item.get("id"))
            time.sleep(1)

    # Save all current bansos IDs if initial run so we mark existing ones as posted
    if is_initial_run:
        for b in bansos_items:
            posted_ids.add(b.get("id"))

    save_posted_ids(posted_ids)

if __name__ == "__main__":
    fetch_and_notify()
