"""Tier & Status handlers"""
from aiogram import Router, F
from aiogram.types import Message, CallbackQuery, InlineKeyboardMarkup, InlineKeyboardButton
from aiogram.filters import Command
from app.services.user_service import user_service
from app.services.rate_limiter import rate_limiter
from app.models.user import TIER_MODELS, TIER_PRICES, TIER_LIMITS

router = Router()


@router.message(Command("status"))
async def cmd_status(message: Message):
    user = await user_service.get_user_by_telegram_id(message.from_user.id)
    
    if not user:
        user = await user_service.get_or_create_user(
            message.from_user.id, message.from_user.username, message.from_user.first_name
        )
    
    expiry = user.tier_expires_at.strftime("%d %b %Y %H:%M") if user.tier_expires_at else "—"
    
    status_text = (
        f"📊 <b>Status Akun Kamu</b>\n\n"
        f"👤 <b>User:</b> {message.from_user.first_name}\n"
        f"🏷️ <b>Tier:</b> <code>{user.tier.upper()}</code>\n"
        f"📅 <b>Berlaku hingga:</b> {expiry}\n\n"
        f"📈 <b>Penggunaan Hari Ini:</b>\n"
        f"  • Terpakai: <code>{user.daily_requests_used}</code>\n"
        f"  • Limit: <code>{user.daily_requests_limit}</code>\n"
        f"  • Sisa: <code>{user.remaining_requests}</code>\n\n"
        f"📊 <b>Total Stats:</b>\n"
        f"  • Total requests: <code>{user.total_requests}</code>\n"
        f"  • Total tokens: <code>{user.total_tokens:,}</code>\n"
        f"  • Model favorit: <code>{user.preferred_model or 'default'}</code>\n\n"
        f"💡 Ketik /upgrade untuk upgrade tier!"
    )
    
    await message.answer(status_text, parse_mode="HTML")


@router.message(Command("models"))
async def cmd_models(message: Message):
    user = await user_service.get_user_by_telegram_id(message.from_user.id)
    tier = user.tier if user else "free"
    
    available = TIER_MODELS.get(tier, [])
    all_models = ["gpt-3.5-turbo", "gpt-4o-mini", "gpt-4o", "claude-3-haiku", "claude-3.5-sonnet", "llama-3-8b", "llama-3-70b"]
    
    text = f"🤖 <b>Model Tersedia</b> (Tier: {tier.upper()})\n\n"
    
    for model in all_models:
        if model in available:
            text += f"✅ <code>{model}</code>\n"
        else:
            text += f"🔒 <code>{model}</code> — perlu upgrade\n"
    
    text += (
        f"\n💡 Ketik <code>/model [nama]</code> untuk memilih model.\n"
        f"Contoh: <code>/model gpt-4o-mini</code>"
    )
    
    await message.answer(text, parse_mode="HTML")


@router.message(Command("model"))
async def cmd_set_model(message: Message):
    args = message.text.split(maxsplit=1)
    
    if len(args) < 2:
        await message.answer(
            "⚠️ Gunakan: <code>/model [nama_model]</code>\n"
            "Ketik /models untuk melihat daftar model.",
            parse_mode="HTML",
        )
        return
    
    model_name = args[1].strip()
    user = await user_service.get_user_by_telegram_id(message.from_user.id)
    
    if not user:
        await message.answer("❌ User tidak ditemukan. Ketik /start dulu.")
        return
    
    if not rate_limiter.is_model_allowed(user.tier, model_name):
        await message.answer(
            f"🔒 Model <code>{model_name}</code> tidak tersedia di tier {user.tier.upper()}.\n"
            f"Ketik /upgrade untuk akses model premium.",
            parse_mode="HTML",
        )
        return
    
    await user_service.set_preferred_model(message.from_user.id, model_name)
    await message.answer(
        f"✅ Model diubah ke <code>{model_name}</code>!\n"
        f"Semua chat berikutnya akan menggunakan model ini.",
        parse_mode="HTML",
    )


@router.message(Command("upgrade"))
async def cmd_upgrade(message: Message):
    keyboard = InlineKeyboardMarkup(inline_keyboard=[
        [InlineKeyboardButton(text="⭐ Basic — Rp 49.000", callback_data="upgrade_basic")],
        [InlineKeyboardButton(text="💎 Premium — Rp 149.000", callback_data="upgrade_premium")],
        [InlineKeyboardButton(text="🏢 Enterprise — Rp 499.000", callback_data="upgrade_enterprise")],
    ])
    
    user = await user_service.get_user_by_telegram_id(message.from_user.id)
    tier = user.tier if user else "free"
    
    text = (
        f"🚀 <b>Upgrade Tier</b>\n\n"
        f"Tier kamu saat ini: <b>{tier.upper()}</b>\n\n"
        f"📋 <b>Pilihan Upgrade:</b>\n\n"
        f"⭐ <b>BASIC — Rp 49.000/bulan</b>\n"
        f"  • 200 request/hari\n"
        f"  • + GPT-4o-mini, Claude-3 Haiku\n\n"
        f"💎 <b>PREMIUM — Rp 149.000/bulan</b>\n"
        f"  • 1000 request/hari\n"
        f"  • + GPT-4o, Claude-3.5 Sonnet\n\n"
        f"🏢 <b>ENTERPRISE — Rp 499.000/bulan</b>\n"
        f"  • Unlimited requests\n"
        f"  • Semua model tersedia\n\n"
        f"Pilih tier di bawah untuk melanjutkan pembayaran:"
    )
    
    await message.answer(text, parse_mode="HTML", reply_markup=keyboard)


@router.callback_query(F.data.startswith("upgrade_"))
async def process_upgrade_selection(callback: CallbackQuery):
    tier = callback.data.replace("upgrade_", "")
    
    if tier not in TIER_PRICES or tier == "free":
        await callback.answer("Pilihan tidak valid", show_alert=True)
        return
    
    await callback.answer()
    
    from app.services.payment_service import payment_service
    result = await payment_service.create_transaction(callback.from_user.id, tier)
    
    if result["success"]:
        # Kirim info pembayaran
        amount_formatted = f"Rp {result['amount']:,.0f}".replace(",", ".")
        
        keyboard = InlineKeyboardMarkup(inline_keyboard=[
            [InlineKeyboardButton(text="✅ Saya Sudah Bayar", callback_data=f"checkpay_{result['external_id']}")],
            [InlineKeyboardButton(text="❌ Batalkan", callback_data=f"cancelpay_{result['external_id']}")],
        ])
        
        await callback.message.answer_photo(
            photo=open(result["qris_image_path"], "rb"),
            caption=(
                f"💰 <b>Pembayaran {tier.upper()}</b>\n\n"
                f"🏷️ Tier: <b>{tier.upper()}</b>\n"
                f"💵 Amount: <b>{amount_formatted}</b>\n"
                f"📋 Order ID: <code>{result['external_id']}</code>\n\n"
                f"📱 <b>Cara Bayar:</b>\n"
                f"1. Scan QR code di atas\n"
                f"2. Bayar sesuai nominal\n"
                f"3. Klik "Saya Sudah Bayar" di bawah\n\n"
                f"⏰ Berlaku 30 menit"
            ),
            parse_mode="HTML",
            reply_markup=keyboard,
        )
    else:
        await callback.message.answer(f"❌ Error: {result['error']}")


@router.callback_query(F.data.startswith("checkpay_"))
async def check_payment(callback: CallbackQuery):
    external_id = callback.data.replace("checkpay_", "")
    
    from app.services.payment_service import payment_service
    result = await payment_service.check_payment(external_id)
    
    if result["success"]:
        if result["status"] == "success":
            await callback.message.answer(
                f"✅ <b>Pembayaran Berhasil!</b>\n\n"
                f"Tier kamu sudah di-upgrade ke <b>{result['tier'].upper()}</b>.\n"
                f"Selamat menikmati fitur premium! 🎉",
                parse_mode="HTML",
            )
        else:
            # Auto-confirm for demo (in production, check with payment gateway)
            confirm = await payment_service.confirm_payment(external_id)
            if confirm["success"]:
                await callback.message.answer(
                    f"✅ <b>Pembayaran Dikonfirmasi!</b>\n\n"
                    f"Tier kamu sudah di-upgrade ke <b>{confirm['tier'].upper()}</b>.\n"
                    f"Selamat menikmati fitur premium! 🎉\n\n"
                    f"Ketik /status untuk cek perubahan.",
                    parse_mode="HTML",
                )
            else:
                await callback.message.answer(
                    f"⏳ Pembayaran belum terdeteksi. Silakan tunggu beberapa menit lalu coba lagi.\n\n"
                    f"Jika sudah bayar, hubungi admin.",
                    parse_mode="HTML",
                )
    
    await callback.answer()


@router.callback_query(F.data.startswith("cancelpay_"))
async def cancel_payment(callback: CallbackQuery):
    await callback.message.edit_text("❌ Pembayaran dibatalkan.")
    await callback.answer()


@router.message(Command("history"))
async def cmd_history(message: Message):
    from app.services.payment_service import payment_service
    
    transactions = await payment_service.get_user_transactions(message.from_user.id)
    
    if not transactions:
        await message.answer("📋 Belum ada riwayat transaksi.")
        return
    
    text = "📋 <b>Riwayat Transaksi</b>\n\n"
    for txn in transactions[:10]:
        status_icon = {"success": "✅", "pending": "⏳", "expired": "⏰", "failed": "❌"}.get(txn.payment_status, "❓")
        amount = f"Rp {txn.amount:,.0f}".replace(",", ".")
        date = txn.created_at.strftime("%d/%m/%Y %H:%M")
        text += f"{status_icon} {txn.tier.upper()} — {amount} — {date}\n"
    
    await message.answer(text, parse_mode="HTML")
