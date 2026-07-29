"""Rate Limiter Service"""
from datetime import datetime, date
from app.services.user_service import user_service
from app.models.user import TIER_LIMITS, TIER_MODELS


class RateLimiter:
    """Rate limiting berdasarkan tier"""
    
    async def check_rate_limit(self, telegram_id: int) -> dict:
        """Cek apakah user masih dalam batas rate limit"""
        user = await user_service.get_user_by_telegram_id(telegram_id)
        
        if not user:
            return {"allowed": False, "reason": "User tidak ditemukan"}
        
        if user.is_banned:
            return {"allowed": False, "reason": "🚫 Akun kamu telah diblokir."}
        
        if user.daily_requests_used >= user.daily_requests_limit:
            return {
                "allowed": False,
                "reason": f"📊 Limit harian habis! ({user.daily_requests_used}/{user.daily_requests_limit})",
                "upgrade_needed": True,
                "current_tier": user.tier,
            }
        
        return {
            "allowed": True,
            "remaining": user.daily_requests_limit - user.daily_requests_used,
            "limit": user.daily_requests_limit,
            "tier": user.tier,
        }
    
    def get_models_for_tier(self, tier: str) -> list:
        """Ambil daftar model yang tersedia untuk tier"""
        return TIER_MODELS.get(tier, TIER_MODELS["free"])
    
    def is_model_allowed(self, tier: str, model: str) -> bool:
        """Cek apakah model tersedia untuk tier"""
        return model in self.get_models_for_tier(tier)


rate_limiter = RateLimiter()
