"""User Service - Manajemen user"""
from datetime import datetime, timedelta
from sqlalchemy import select, func, update
from sqlalchemy.ext.asyncio import AsyncSession
from app.database import async_session
from app.models.user import User, TIER_LIMITS


class UserService:
    """Service untuk manajemen user"""
    
    async def get_or_create_user(
        self, telegram_id: int, username: str = None, first_name: str = None
    ) -> User:
        """Ambil user atau buat baru jika belum ada"""
        async with async_session() as session:
            result = await session.execute(
                select(User).where(User.telegram_id == telegram_id)
            )
            user = result.scalar_one_or_none()
            
            if user:
                if username and user.username != username:
                    user.username = username
                if first_name and user.first_name != first_name:
                    user.first_name = first_name
                await session.commit()
                return user
            
            user = User(
                telegram_id=telegram_id,
                username=username,
                first_name=first_name,
                tier="free",
                daily_requests_limit=TIER_LIMITS["free"],
            )
            session.add(user)
            await session.commit()
            await session.refresh(user)
            return user
    
    async def get_user_by_telegram_id(self, telegram_id: int) -> User | None:
        async with async_session() as session:
            result = await session.execute(
                select(User).where(User.telegram_id == telegram_id)
            )
            return result.scalar_one_or_none()
    
    async def increment_usage(self, telegram_id: int, tokens: int = 0) -> bool:
        """Increment usage counter. Returns False if limit exceeded."""
        async with async_session() as session:
            result = await session.execute(
                select(User).where(User.telegram_id == telegram_id)
            )
            user = result.scalar_one_or_none()
            
            if not user or user.is_banned:
                return False
            
            if user.daily_requests_used >= user.daily_requests_limit:
                return False
            
            user.daily_requests_used += 1
            user.total_requests += 1
            user.total_tokens += tokens
            await session.commit()
            return True
    
    async def reset_daily_limits(self):
        """Reset semua daily counters (jalankan setiap tengah malam)"""
        async with async_session() as session:
            await session.execute(
                update(User).values(daily_requests_used=0)
            )
            await session.commit()
    
    async def upgrade_tier(self, telegram_id: int, tier: str, days: int = 30):
        """Upgrade tier user"""
        async with async_session() as session:
            result = await session.execute(
                select(User).where(User.telegram_id == telegram_id)
            )
            user = result.scalar_one_or_none()
            
            if not user:
                return False
            
            user.tier = tier
            user.daily_requests_limit = TIER_LIMITS.get(tier, 20)
            user.tier_expires_at = datetime.utcnow() + timedelta(days=days)
            await session.commit()
            return True
    
    async def check_tier_expiry(self):
        """Cek dan reset tier yang sudah expired"""
        async with async_session() as session:
            result = await session.execute(
                select(User).where(
                    User.tier != "free",
                    User.tier_expires_at < datetime.utcnow(),
                )
            )
            expired_users = result.scalars().all()
            
            for user in expired_users:
                user.tier = "free"
                user.daily_requests_limit = TIER_LIMITS["free"]
                user.tier_expires_at = None
            
            if expired_users:
                await session.commit()
            
            return len(expired_users)
    
    async def get_all_users(self, page: int = 1, per_page: int = 50):
        """Ambil semua user dengan pagination"""
        async with async_session() as session:
            offset = (page - 1) * per_page
            result = await session.execute(
                select(User).order_by(User.created_at.desc()).offset(offset).limit(per_page)
            )
            users = result.scalars().all()
            
            count_result = await session.execute(select(func.count(User.id)))
            total = count_result.scalar()
            
            return users, total
    
    async def ban_user(self, telegram_id: int, ban: bool = True):
        async with async_session() as session:
            result = await session.execute(
                select(User).where(User.telegram_id == telegram_id)
            )
            user = result.scalar_one_or_none()
            if user:
                user.is_banned = ban
                await session.commit()
                return True
            return False
    
    async def set_preferred_model(self, telegram_id: int, model: str):
        async with async_session() as session:
            result = await session.execute(
                select(User).where(User.telegram_id == telegram_id)
            )
            user = result.scalar_one_or_none()
            if user:
                user.preferred_model = model
                await session.commit()
                return True
            return False
    
    async def get_stats(self) -> dict:
        """Ambil statistik untuk dashboard"""
        async with async_session() as session:
            total = (await session.execute(select(func.count(User.id)))).scalar()
            active_today = (await session.execute(
                select(func.count(User.id)).where(User.daily_requests_used > 0)
            )).scalar()
            
            tier_counts = {}
            for tier in ["free", "basic", "premium", "enterprise"]:
                count = (await session.execute(
                    select(func.count(User.id)).where(User.tier == tier)
                )).scalar()
                tier_counts[tier] = count
            
            total_requests = (await session.execute(
                select(func.sum(User.total_requests))
            )).scalar() or 0
            
            return {
                "total_users": total,
                "active_today": active_today,
                "tier_counts": tier_counts,
                "total_requests": total_requests,
            }


user_service = UserService()
