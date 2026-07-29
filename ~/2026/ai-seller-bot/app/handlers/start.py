"""Start & Help handlers"""
from aiogram import Router, F
from aiogram.types import Message
from aiogram.filters import CommandStart, Command
from app.services.user_service import user_service

router = Router()


@router.message(CommandStart())
async def cmd_start(message: Message):
    user = await user_service.get_or_create_user(
        telegram_id=message.from_user.id,
        username=message.from_user.username,
        first_name=message.from_user.first_name,
    )
    
    welcome = (
        f"🤖 <b>Selamat datang, {message.from_user.first_name}!</b>\n\n"
        f"Saya adalah AI Assistant yang powered by berbagai model AI premium.\n\n"
        f"📌 <b>Yang bisa saya lakukan:</b>\n"
        f"• Menjawab pertanyaan dengan AI\n"
        f"• Membantu menulis, coding, analisis\n"
        f"• Dan masih banyak lagi!\n\n"
        f"🆓 <b>Tier kamu:</b> <code>FREE</code>\n"
        f"📊 <b>Limit hari ini:</b> <code>{user.remaining_requests}/{user.daily_requests_limit}</code>\n\n"
        f"💡 Ketik langsung pesanmu untuk mulai chat!\n"
        f"Ketik /help untuk bantuan lengkap."
    )
    
    await message.answer(welcome, parse_mode="HTML")


@router.message(Command("help"))
async def cmd_help(message: Message):
    help_text = (
        "📖 <b>Panduan Penggunaan</b>\n\n"
        "<b>💬 Chat:</b>\n"
        "Ketik langsung pesanmu, saya akan merespons dengan AI.\n\n"
        "<b>📋 Commands:</b>\n"
        "/start — Mulai bot\n"
        "/help — Bantuan ini\n"
        "/status — Cek tier & limit kamu\n"
        "/models — Lihat model AI tersedia\n"
        "/model [nama] — Pilih model AI favorit\n"
        "/upgrade — Upgrade tier untuk lebih banyak fitur\n"
        "/history — Riwayat pembelian\n"
        "/reset — Reset conversation history\n\n"
        "<b>🆓 Tier FREE:</b>\n"
        "• 20 request/hari\n"
        "• Model: GPT-3.5, Llama-3-8B\n\n"
        "<b>⭐ Tier BASIC (Rp 49.000/bulan):</b>\n"
        "• 200 request/hari\n"
        "• Model: + GPT-4o-mini, Claude-3 Haiku\n\n"
        "<b>💎 Tier PREMIUM (Rp 149.000/bulan):</b>\n"
        "• 1000 request/hari\n"
        "• Model: + GPT-4o, Claude-3.5 Sonnet\n\n"
        "<b>🏢 Tier ENTERPRISE (Rp 499.000/bulan):</b>\n"
        "• Unlimited requests\n"
        "• Semua model tersedia\n\n"
        "Ketik /upgrade untuk mulai upgrade! 🚀"
    )
    
    await message.answer(help_text, parse_mode="HTML")
