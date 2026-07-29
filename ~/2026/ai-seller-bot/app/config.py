"""AI Seller Bot - Configuration"""
import os
from pathlib import Path
from pydantic_settings import BaseSettings
from typing import List


class Settings(BaseSettings):
    # Bot
    BOT_TOKEN: str = ""
    
    # 9Router
    NINEROUTER_URL: str = "http://localhost:20128"
    NINEROUTER_API_KEY: str = ""
    
    # Database
    DATABASE_URL: str = "postgresql+asyncpg://cahya:password@localhost:5432/ai_seller"
    
    # Admin
    ADMIN_TELEGRAM_IDS: str = ""
    
    # Payment
    QRIS_MERCHANT_ID: str = ""
    QRIS_API_KEY: str = ""
    
    # App
    APP_HOST: str = "0.0.0.0"
    APP_PORT: int = 8080
    DEBUG: bool = True
    
    @property
    def admin_ids(self) -> List[int]:
        if not self.ADMIN_TELEGRAM_IDS:
            return []
        return [int(x.strip()) for x in self.ADMIN_TELEGRAM_IDS.split(",") if x.strip()]
    
    class Config:
        env_file = str(Path(__file__).parent.parent / ".env")
        env_file_encoding = "utf-8"


settings = Settings()
