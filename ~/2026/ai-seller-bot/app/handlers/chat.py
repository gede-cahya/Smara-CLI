"""Chat handler - Proses chat dengan AI"""
from aiogram import Router, F
from aiogram.types import Message
from app.services.user_service import user_service
from app.services.rate_limiter import rate_limiter
from app.services.ai_service import ai_service
from app.models.user import TIER_MODELS

router = Router()

# Conversation history per user (in-memory, reset on restart)
user_histories: dict[int, list] = {}
MAX_HISTORY = 10  # Simpan 10 pesan terakhir


@router.message(F.text & ~F.text.startswith("/"))
async def handle_chat(message: Message):
    telegram_id = message.from_user.id
    text = message.text
    
    if not text or len(text.strip()) == 0:
        return
    
    # Cek rate limit
    rate_check = await rate_limiter.check_rate_limit(telegram_id)
    
    if not rate_check["allowed"]:
        if rate_check.get("upgrade_needed"):
            await message.answer(
                f"📊 {rate_check['reason']}\n\n"
                f"Ketik /upgrade untuk upgrade tier kamu!\n"
                f"Limit akan direset besok pagi.",
                parse_mode="HTML",
            )
        else:
            await message.answer(rate_check["reason"])
        return
    
    # Ambil user untuk model preference
    user = await user_service.get_user_by_telegram_id(telegram_id)
    if not user:
        user = await user_service.get_or_create_user(telegram_id, message.from_user.username, message.from_user.first_name)
    
    # Tentukan model
    model = user.preferred_model or TIER_MODELS.get(user.tier, ["gpt-3.5-turbo"])[0]
    
    # Cek model valid
    if not rate_limiter.is_model_allowed(user.tier, model):
        model = TIER_MODELS.get(user.tier, ["gpt-3.5-turbo"])[0]
    
    # Ambil history
    history = user_histories.get(telegram_id, [])
    
    # Kirim typing indicator
    await message.bot.send_chat_action(message.chat.id, "typing")
    
    # Request ke AI
    result = await ai_service.chat(
        message=text,
        model=model,
        history=history,
    )
    
    if result["success"]:
        # Increment usage
        await user_service.increment_usage(telegram_id, result["total_tokens"])
        
        # Simpan history
        if telegram_id not in user_histories:
            user_histories[telegram_id] = []
        
        user_histories[telegram_id].append({"role": "user", "content": text})
        user_histories[telegram_id].append({"role": "assistant", "content": result["content"]})
        
        # Trim history
        if len(user_histories[telegram_id]) > MAX_HISTORY * 2:
            user_histories[telegram_id] = user_histories[telegram_id][-MAX_HISTORY * 2:]
        
        # Kirim response
        response_text = result["content"]
        
        # Tambah info model & limit di footer
        remaining = rate_check["remaining"] - 1
        footer = f"\n\n<i>🤖 {model} | 📊 {remaining} request tersisa hari ini</i>"
        
        # Split jika terlalu panjang (Telegram limit 4096)
        if len(response_text) + len(footer) > 4000:
            # Kirim response panjang dulu
            for i in range(0, len(response_text), 4000):
                chunk = response_text[i:i+4000]
                if i + 4000 >= len(response_text):
                    await message.answer(chunk + footer, parse_mode="HTML")
                else:
                    await message.answer(chunk, parse_mode="HTML")
        else:
            await message.answer(response_text + footer, parse_mode="HTML")
        
        # Log usage (async, non-blocking)
        from app.models.usage_log import UsageLog
        from app.database import async_session
        async with async_session() as session:
            log = UsageLog(
                user_id=user.id,
                telegram_id=telegram_id,
                model=model,
                prompt_tokens=result["prompt_tokens"],
                completion_tokens=result["completion_tokens"],
                total_tokens=result["total_tokens"],
                response_time_ms=result["response_time_ms"],
                status="success",
            )
            session.add(log)
            await session.commit()
    else:
        await message.answer(f"{result['error']}\n\n<i>Coba ganti model dengan /model</i>", parse_mode="HTML")
