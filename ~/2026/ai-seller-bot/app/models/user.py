"""User model"""
from datetime import datetime
from sqlalchemy import BigInteger, String, Integer, DateTime, Float, Boolean, func
from sqlalchemy.orm import Mapped, mapped_column
from app.database import Base


class User(Base):
    __tablename__ = "users"
    
    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    telegram_id: Mapped[int] = mapped_column(BigInteger, unique=True, nullable=False, index=True)
    username: Mapped[str] = mapped_column(String(255), nullable=True)
    first_name: Mapped[str] = mapped_column(String(255), nullable=True)
    tier: Mapped[str] = mapped_column(String(20), default="free")
    tier_expires_at: Mapped[datetime] = mapped_column(DateTime, nullable=True)
    daily_requests_used: Mapped[int] = mapped_column(Integer, default=0)
    daily_requests_limit: Mapped[int] = mapped_column(Integer, default=20)
    total_requests: Mapped[int] = mapped_column(Integer, default=0)
    total_tokens: Mapped[int] = mapped_column(Integer, default=0)
    is_banned: Mapped[bool] = mapped_column(Boolean, default=False)
    preferred_model: Mapped[str] = mapped_column(String(50), nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime, server_default=func.now())
    updated_at: Mapped[datetime] = mapped_column(DateTime, server_default=func.now(), onupdate=func.now())
    
    def __repr__(self):
        return f"<User {self.telegram_id} tier={self.tier}>"
    
    @property
    def is_premium(self) -> bool:
        return self.tier in ("basic", "premium", "enterprise")
    
    @property
    def remaining_requests(self) -> int:
        return max(0, self.daily_requests_limit - self.daily_requests_used)


TIER_LIMITS = {
    "free": 20,
    "basic": 200,
    "premium": 1000,
    "enterprise": 999999,
}

TIER_MODELS = {
    "free": ["gpt-3.5-turbo", "llama-3-8b"],
    "basic": ["gpt-3.5-turbo", "gpt-4o-mini", "claude-3-haiku", "llama-3-8b"],
    "premium": ["gpt-3.5-turbo", "gpt-4o-mini", "gpt-4o", "claude-3-haiku", "claude-3.5-sonnet", "llama-3-8b"],
    "enterprise": ["gpt-3.5-turbo", "gpt-4o-mini", "gpt-4o", "claude-3-haiku", "claude-3.5-sonnet", "llama-3-8b", "llama-3-70b"],
}

TIER_PRICES = {
    "free": 0,
    "basic": 49000,
    "premium": 149000,
    "enterprise": 499000,
}
