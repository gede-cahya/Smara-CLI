"""Usage Log model"""
from datetime import datetime
from sqlalchemy import BigInteger, String, Integer, DateTime, Float, Text, func
from sqlalchemy.orm import Mapped, mapped_column
from app.database import Base


class UsageLog(Base):
    __tablename__ = "usage_logs"
    
    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    user_id: Mapped[int] = mapped_column(Integer, nullable=False, index=True)
    telegram_id: Mapped[int] = mapped_column(BigInteger, nullable=False, index=True)
    model: Mapped[str] = mapped_column(String(50), nullable=False)
    prompt_tokens: Mapped[int] = mapped_column(Integer, default=0)
    completion_tokens: Mapped[int] = mapped_column(Integer, default=0)
    total_tokens: Mapped[int] = mapped_column(Integer, default=0)
    response_time_ms: Mapped[int] = mapped_column(Integer, default=0)
    status: Mapped[str] = mapped_column(String(20), default="success")
    error_message: Mapped[str] = mapped_column(Text, nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime, server_default=func.now())
    
    def __repr__(self):
        return f"<UsageLog user={self.telegram_id} model={self.model}>"
