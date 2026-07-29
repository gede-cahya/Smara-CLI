"""Payment Service - QRIS & Transaction management"""
import uuid
import qrcode
import io
import os
from datetime import datetime, timedelta
from pathlib import Path
from sqlalchemy import select
from app.database import async_session
from app.models.transaction import Transaction
from app.services.user_service import user_service
from app.models.user import TIER_PRICES


class PaymentService:
    """Service untuk manajemen pembayaran QRIS"""
    
    QRIS_DIR = Path(__file__).parent.parent.parent / "data" / "qris"
    
    def __init__(self):
        self.QRIS_DIR.mkdir(parents=True, exist_ok=True)
    
    def generate_qris_payload(self, amount: float, external_id: str) -> str:
        """Generate QRIS payload string (static QRIS format)
        
        NOTE: Ini adalah placeholder. Untuk production, 
        integrasikan dengan payment gateway (Midtrans, Tripay, dll)
        atau generate QRIS statis dari merchant QRIS kamu.
        """
        # Format QRIS sederhana (akan diisi dengan QRIS statis merchant)
        qris_string = (
            f"00020101021226{len(self._format_amount(amount)):02d}"
            f"{self._format_amount(amount)}5204000053033605404"
            f"{amount:.0f}5802ID5920AI SELLER BOT6007Jakarta"
            f"62{len(external_id):02d}{external_id}6304"
        )
        return qris_string
    
    def _format_amount(self, amount: float) -> str:
        return f"{amount:.0f}"
    
    async def create_transaction(self, telegram_id: int, tier: str) -> dict:
        """Buat transaksi baru dan generate QRIS"""
        amount = TIER_PRICES.get(tier, 0)
        if amount == 0:
            return {"success": False, "error": "Tier free tidak perlu pembayaran"}
        
        external_id = f"TXN-{uuid.uuid4().hex[:12].upper()}"
        expires_at = datetime.utcnow() + timedelta(minutes=30)
        
        user = await user_service.get_user_by_telegram_id(telegram_id)
        if not user:
            return {"success": False, "error": "User tidak ditemukan"}
        
        # Generate QR code
        qris_payload = self.generate_qris_payload(amount, external_id)
        qr = qrcode.QRCode(version=1, box_size=10, border=4)
        qr.add_data(qris_payload)
        qr.make(fit=True)
        img = qr.make_image(fill_color="black", back_color="white")
        
        # Save QR image
        img_path = self.QRIS_DIR / f"{external_id}.png"
        img.save(str(img_path))
        
        # Simpan transaksi
        async with async_session() as session:
            txn = Transaction(
                user_id=user.id,
                telegram_id=telegram_id,
                tier=tier,
                amount=amount,
                qris_payload=qris_payload,
                qris_image_path=str(img_path),
                external_id=external_id,
                payment_status="pending",
                expires_at=expires_at,
            )
            session.add(txn)
            await session.commit()
            await session.refresh(txn)
        
        return {
            "success": True,
            "external_id": external_id,
            "amount": amount,
            "tier": tier,
            "qris_image_path": str(img_path),
            "expires_at": expires_at,
        }
    
    async def check_payment(self, external_id: str) -> dict:
        """Cek status pembayaran"""
        async with async_session() as session:
            result = await session.execute(
                select(Transaction).where(Transaction.external_id == external_id)
            )
            txn = result.scalar_one_or_none()
            
            if not txn:
                return {"success": False, "error": "Transaksi tidak ditemukan"}
            
            return {
                "success": True,
                "status": txn.payment_status,
                "tier": txn.tier,
                "amount": txn.amount,
            }
    
    async def confirm_payment(self, external_id: str, admin_id: int = None) -> dict:
        """Konfirmasi pembayaran (manual by admin atau webhook)"""
        async with async_session() as session:
            result = await session.execute(
                select(Transaction).where(Transaction.external_id == external_id)
            )
            txn = result.scalar_one_or_none()
            
            if not txn:
                return {"success": False, "error": "Transaksi tidak ditemukan"}
            
            if txn.payment_status != "pending":
                return {"success": False, "error": f"Status sudah {txn.payment_status}"}
            
            txn.payment_status = "success"
            txn.paid_at = datetime.utcnow()
            txn.notes = f"Confirmed by admin {admin_id}" if admin_id else "Auto-confirmed"
            await session.commit()
            
            # Upgrade tier user
            success = await user_service.upgrade_tier(txn.telegram_id, txn.tier, days=30)
            
            return {
                "success": True,
                "telegram_id": txn.telegram_id,
                "tier": txn.tier,
                "amount": txn.amount,
                "tier_upgraded": success,
            }
    
    async def get_pending_transactions(self) -> list:
        """Ambil semua transaksi pending"""
        async with async_session() as session:
            result = await session.execute(
                select(Transaction).where(
                    Transaction.payment_status == "pending"
                ).order_by(Transaction.created_at.desc())
            )
            return result.scalars().all()
    
    async def get_user_transactions(self, telegram_id: int) -> list:
        """Ambil riwayat transaksi user"""
        async with async_session() as session:
            result = await session.execute(
                select(Transaction).where(
                    Transaction.telegram_id == telegram_id
                ).order_by(Transaction.created_at.desc())
            )
            return result.scalars().all()
    
    async def expire_old_transactions(self):
        """Expire transaksi yang melewati batas waktu"""
        async with async_session() as session:
            result = await session.execute(
                select(Transaction).where(
                    Transaction.payment_status == "pending",
                    Transaction.expires_at < datetime.utcnow(),
                )
            )
            expired = result.scalars().all()
            
            for txn in expired:
                txn.payment_status = "expired"
            
            if expired:
                await session.commit()
            
            return len(expired)
    
    async def get_revenue_stats(self) -> dict:
        """Ambil statistik revenue"""
        async with async_session() as session:
            from sqlalchemy import func
            
            total_revenue = (await session.execute(
                select(func.sum(Transaction.amount)).where(
                    Transaction.payment_status == "success"
                )
            )).scalar() or 0
            
            today_start = datetime.utcnow().replace(hour=0, minute=0, second=0, microsecond=0)
            today_revenue = (await session.execute(
                select(func.sum(Transaction.amount)).where(
                    Transaction.payment_status == "success",
                    Transaction.paid_at >= today_start,
                )
            )).scalar() or 0
            
            total_transactions = (await session.execute(
                select(func.count(Transaction.id)).where(
                    Transaction.payment_status == "success"
                )
            )).scalar()
            
            pending_count = (await session.execute(
                select(func.count(Transaction.id)).where(
                    Transaction.payment_status == "pending"
                )
            )).scalar()
            
            return {
                "total_revenue": total_revenue,
                "today_revenue": today_revenue,
                "total_transactions": total_transactions,
                "pending_payments": pending_count,
            }


payment_service = PaymentService()
