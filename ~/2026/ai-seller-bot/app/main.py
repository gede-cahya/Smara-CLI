"""AI Seller Bot - Main Entry Point"""
import asyncio
import logging
from aiogram import Bot, Dispatcher
from aiogram.enums import ParseMode
from aiogram.client.default import DefaultBotProperties
from apscheduler.schedulers.asyncio import AsyncIOScheduler

from app.config import settings
from app.database import init_db
from app.handlers import start, chat, tier, admin
from app.services.user_service import user_service
from app.services.payment_service import payment_service
from app.services.ai_service import ai_service

# Logging
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
)
logger = logging.getLogger(__name__)


async def scheduled_tasks():
    """Tasks yang berjalan periodik"""
    # Reset daily limits setiap jam 00:00
    await user_service.reset_daily_limits()
    logger.info("Daily limits reset completed")
    
    # Expire old transactions
    expired = await payment_service.expire_old_transactions()
    if expired:
        logger.info(f"Expired {expired} old transactions")
    
    # Check tier expiry
    expired_tiers = await user_service.check_tier_expiry()
    if expired_tiers:
        logger.info(f"Reset {expired_tiers} expired tiers")


async def main():
    logger.info("Starting AI Seller Bot...")
    
    # Validate config
    if not settings.BOT_TOKEN or settings.BOT_TOKEN == "YOUR_BOT_TOKEN_HERE":
        logger.error("BOT_TOKEN belum di-set! Edit file .env")
        print("\n" + "=" * 50)
        print("ERROR: BOT_TOKEN belum di-set!")
        print("Edit file ~/2026/ai-seller-bot/.env")
        print("Isi BOT_TOKEN dari @BotFather")
        print("=" * 50 + "\n")
        return
    
    # Init database
    logger.info("Initializing database...")
    await init_db()
    logger.info("Database initialized")
    
    # Create bot & dispatcher
    bot = Bot(
        token=settings.BOT_TOKEN,
        default=DefaultBotProperties(parse_mode=ParseMode.HTML),
    )
    dp = Dispatcher()
    
    # Register routers
    dp.include_router(start.router)
    dp.include_router(tier.router)
    dp.include_router(admin.router)
    dp.include_router(chat.router)  # Chat harus terakhir (catch-all)
    
    # Setup scheduler
    scheduler = AsyncIOScheduler()
    scheduler.add_job(scheduled_tasks, "cron", hour=0, minute=0)
    scheduler.start()
    
    # Get bot info
    bot_info = await bot.get_me()
    logger.info(f"Bot started: @{bot_info.username} ({bot_info.first_name})")
    
    # Notify admins
    for admin_id in settings.admin_ids:
        try:
            await bot.send_message(
                admin_id,
                f"🤖 <b>Bot Started!</b>\n\n"
                f"Username: @{bot_info.username}\n"
                f"Status: Running ✅\n"
                f"Ketik /admin untuk dashboard.",
                parse_mode="HTML",
            )
        except Exception as e:
            logger.warning(f"Failed to notify admin {admin_id}: {e}")
    
    # Start polling
    try:
        logger.info("Starting polling...")
        await dp.start_polling(bot, allowed_updates=dp.resolve_used_update_types())
    finally:
        logger.info("Shutting down...")
        await ai_service.close()
        await bot.session.close()
        scheduler.shutdown()


if __name__ == "__main__":
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        print("\nBot stopped.")
