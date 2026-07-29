"""Admin handlers"""
from aiogram import Router, F
from aiogram.types import Message
from aiogram.filters import Command
from app.config import settings
from app.services.user_service import user_service
from app.services.payment_service import payment_service

router = Router()


def is_admin(user_id: int) -> bool:
    return user_id in settings.admin_ids


@router.message(Command("admin"))
async def cmd_admin(message: Message):
    if not is_admin(message.from_user.id):
        await message.answer("❌ Kamu bukan admin.")
        return
    
    stats = await user_service.get_stats()
    revenue = await payment_service.get_revenue_stats()
    
    text = (
        f"🔧 <b>Admin Dashboard</b>\n\n"
        f"👥 <b>Users:</b>\n"
        f"  • Total: {stats['total_users']}\n"
        f"  • Aktif hari ini: {stats['active_today']}\n"
        f"  • Free: {stats['tier_counts']['free']}\n"
        f"  • Basic: {stats['tier_counts']['basic']}\n"
        f"  • Premium: {stats['tier_counts']['premium']}\n"
        f"  • Enterprise: {stats['tier_counts']['enterprise']}\n\n"
        f"💰 <b>Revenue:</b>\n"
        f"  • Total: Rp {revenue['total_revenue']:,.0f}\n"
        f"  • Hari ini: Rp {revenue['today_revenue']:,.0f}\n"
        f"  • Transaksi sukses: {revenue['total_transactions']}\n"
        f"  • Pending: {revenue['pending_payments']}\n\n"
        f"📊 Total requests: {stats['total_requests']}\n\n"
        f"<b>Commands:</b>\n"
        f"/admin — Dashboard\n"
        f"/users [page] — List users\n"
        f"/pending — Transaksi pending\n"
        f"/confirm [order_id] — Konfirmasi pembayaran\n"
        f"/ban [user_id] — Ban user\n"
        f"/unban [user_id] — Unban user\n"
        f"/broadcast [pesan] — Broadcast ke semua user\n"
        f"/resetlimits — Reset daily limits"
    )
    
    await message.answer(text, parse_mode="HTML")


@router.message(Command("users"))
async def cmd_users(message: Message):
    if not is_admin(message.from_user.id):
        return
    
    args = message.text.split()
    page = int(args[1]) if len(args) > 1 else 1
    
    users, total = await user_service.get_all_users(page=page, per_page=10)
    total_pages = (total + 9) // 10
    
    text = f"👥 <b>Users (Page {page}/{total_pages}, Total: {total})</b>\n\n"
    
    for user in users:
        ban_icon = "🚫" if user.is_banned else "✅"
        text += (
            f"{ban_icon} <code>{user.telegram_id}</code> — "
            f"{user.first_name or user.username or 'N/A'} — "
            f"{user.tier.upper()} — "
            f"{user.total_requests} req\n"
        )
    
    await message.answer(text, parse_mode="HTML")


@router.message(Command("pending"))
async def cmd_pending(message: Message):
    if not is_admin(message.from_user.id):
        return
    
    transactions = await payment_service.get_pending_transactions()
    
    if not transactions:
        await message.answer("✅ Tidak ada transaksi pending.")
        return
    
    text = f"⏳ <b>Pending Transactions ({len(transactions)})</b>\n\n"
    
    for txn in transactions[:20]:
        text += (
            f"📋 <code>{txn.external_id}</code>\n"
            f"  User: <code>{txn.telegram_id}</code>\n"
            f"  Tier: {txn.tier.upper()} — Rp {txn.amount:,.0f}\n"
            f"  Created: {txn.created_at.strftime('%d/%m %H:%M')}\n\n"
        )
    
    text += "Gunakan /confirm [order_id] untuk konfirmasi."
    await message.answer(text, parse_mode="HTML")


@router.message(Command("confirm"))
async def cmd_confirm(message: Message):
    if not is_admin(message.from_user.id):
        return
    
    args = message.text.split(maxsplit=1)
    if len(args) < 2:
        await message.answer("⚠️ Gunakan: /confirm [order_id]")
        return
    
    order_id = args[1].strip()
    result = await payment_service.confirm_payment(order_id, admin_id=message.from_user.id)
    
    if result["success"]:
        amount = f"Rp {result['amount']:,.0f}".replace(",", ".")
        await message.answer(
            f"✅ Pembayaran dikonfirmasi!\n\n"
            f"User: <code>{result['telegram_id']}</code>\n"
            f"Tier: {result['tier'].upper()}\n"
            f"Amount: {amount}",
            parse_mode="HTML",
        )
        
        # Notify user
        try:
            await message.bot.send_message(
                result["telegram_id"],
                f"✅ <b>Pembayaran Dikonfirmasi Admin!</b>\n\n"
                f"Tier kamu sudah di-upgrade ke <b>{result['tier'].upper()}</b>.\n"
                f"Selamat menikmati fitur premium! 🎉",
                parse_mode="HTML",
            )
        except Exception:
            pass
    else:
        await message.answer(f"❌ {result['error']}")


@router.message(Command("ban"))
async def cmd_ban(message: Message):
    if not is_admin(message.from_user.id):
        return
    
    args = message.text.split()
    if len(args) < 2:
        await message.answer("⚠️ Gunakan: /ban [telegram_id]")
        return
    
    target_id = int(args[1])
    success = await user_service.ban_user(target_id, True)
    
    if success:
        await message.answer(f"🚫 User <code>{target_id}</code> telah dibanned.", parse_mode="HTML")
    else:
        await message.answer(f"❌ User tidak ditemukan.")


@router.message(Command("unban"))
async def cmd_unban(message: Message):
    if not is_admin(message.from_user.id):
        return
    
    args = message.text.split()
    if len(args) < 2:
        await message.answer("⚠️ Gunakan: /unban [telegram_id]")
        return
    
    target_id = int(args[1])
    success = await user_service.ban_user(target_id, False)
    
    if success:
        await message.answer(f"✅ User <code>{target_id}</code> telah di-unban.", parse_mode="HTML")
    else:
        await message.answer(f"❌ User tidak ditemukan.")


@router.message(Command("broadcast"))
async def cmd_broadcast(message: Message):
    if not is_admin(message.from_user.id):
        return
    
    args = message.text.split(maxsplit=1)
    if len(args) < 2:
        await message.answer("⚠️ Gunakan: /broadcast [pesan]")
        return
    
    broadcast_text = args[1]
    users, total = await user_service.get_all_users(page=1, per_page=9999)
    
    sent = 0
    failed = 0
    
    for user in users:
        try:
            await message.bot.send_message(user.telegram_id, f"📢 <b>Broadcast:</b>\n\n{broadcast_text}", parse_mode="HTML")
            sent += 1
        except Exception:
            failed += 1
    
    await message.answer(f"📢 Broadcast selesai:\n✅ Terkirim: {sent}\n❌ Gagal: {failed}")


@router.message(Command("resetlimits"))
async def cmd_reset_limits(message: Message):
    if not is_admin(message.from_user.id):
        return
    
    await user_service.reset_daily_limits()
    await message.answer("✅ Semua daily limits telah di-reset!")
