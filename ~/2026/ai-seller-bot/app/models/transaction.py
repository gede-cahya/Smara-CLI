"""Transaction model"""
from datetime import datetime
from sqlalchemy import BigInteger, String, Integer, DateTime, Float, Text, func
from sqlalchemy.orm import Mapped, mapped_column
from app.database import Base


class Transaction(Base):
    __tablename__ = "transactions"
    
    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    user_id: Mapped[int] = mapped_column(Integer, nullable=False, index=True)
    telegram_id: Mapped[int] = mapped_column(BigInteger, nullable=False, index=True)
    tier: Mapped[str] = mapped_column(String(20), nullable=False)
    amount: Mapped[float] = mapped_column(Float, nullable=False)
    qris_payload: Mapped[str] = mapped_column(Text, nullable=True)
    qris_image_path: Mapped[str] = mapped_column(String(500), nullable=True)
    payment_status: Mapped[str] = mapped_column(String(20), default="pending")
    external_id: Mapped[str] = mapped_column(String(100), unique=True, nullable=True)
    paid_at: Mapped[datetime] = mapped_column(DateTime, nullable=True)
    expires_at: Mapped[datetime] = mapped_column(DateTime, nullable=True)
    notes: Mapped[str] = mapped_column(Text, nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime, server_default=func.now())
    
    def __repr__(self):
        return f"<Transaction {self.external_id} status={self.payment_status}>"
