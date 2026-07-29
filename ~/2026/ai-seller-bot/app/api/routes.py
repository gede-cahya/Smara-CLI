"""REST API — OpenAI-compatible + Admin Dashboard (FastAPI)"""
import hashlib
import datetime
from typing import List, Optional

from fastapi import FastAPI, Depends, HTTPException, Header
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
import jwt
import httpx
from sqlalchemy import select, func

from app.config import Config
from app.models.db import async_session
from app.models.database import User, ApiKey, Transaction, UsageLog
from app.services.user_service import user_service
from app.services.payment_service import payment_service
from app.services.apikey_service import api_key_service
from app.services.ai_service import ai_service


# ─── FastAPI App ─────────────────────────────────────────
app = FastAPI(title="AI Seller API", version="1.0.0")

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

ADMIN_SECRET = Config.API_SECRET_KEY or "change-me-to-random-string"


# ═══════════════════════════════════════════════════════════
#  HELPERS
# ═══════════════════════════════════════════════════════════

def _hash_key(key: str) -> str:
    return hashlib.sha256(key.encode()).hexdigest()


async def _validate_api_key(authorization: str) -> tuple:
    """Validate API key. Returns (api_key_or_None, user, log_id_or_None)."""
    if not authorization:
        raise HTTPException(status_code=401, detail="Missing Authorization header")

    key = authorization.replace("Bearer ", "").strip()
    if not key:
        raise HTTPException(status_code=401, detail="Empty API key")

    # Master key (from .env NINEROUTER_API_KEY) → enterprise access
    if key == Config.NINEROUTER_API_KEY:
        async with async_session() as session:
            result = await session.execute(
                select(User).where(User.telegram_id.in_(Config.ADMIN_IDS))
            )
            admin_user = result.scalar_one_or_none()
            if admin_user:
                return None, admin_user, None
        # Master key but no admin user yet — still allow
        raise HTTPException(status_code=403, detail="Admin user not registered. Send /start to bot first.")

    # Normal API key
    async with async_session() as session:
        k_hash = _hash_key(key)
        result = await session.execute(
            select(ApiKey).where(ApiKey.key_hash == k_hash, ApiKey.is_active == True)
        )
        api_key = result.scalar_one_or_none()

        if not api_key:
            raise HTTPException(status_code=401, detail="Invalid or revoked API key")

        user = await session.get(User, api_key.user_id)
        if not user or user.is_banned:
            raise HTTPException(status_code=403, detail="User not found or banned")

        if api_key.daily_used >= api_key.daily_limit:
            raise HTTPException(
                status_code=429,
                detail=f"Daily limit reached ({api_key.daily_limit}). Resets at midnight.",
            )

        # Increment usage
        api_key.daily_used += 1
        api_key.total_requests += 1
        api_key.last_used_at = datetime.datetime.utcnow()
        user.total_requests = (user.total_requests or 0) + 1

        log = UsageLog(user_id=user.id, api_key_id=api_key.id, model="pending", tokens_in=0, tokens_out=0)
        session.add(log)
        await session.commit()
        await session.refresh(log)

        return api_key, user, log.id


def _verify_admin_jwt(authorization: str = Header(None)):
    if not authorization:
        raise HTTPException(status_code=401, detail="No token provided")
    token = authorization.replace("Bearer ", "")
    try:
        return jwt.decode(token, ADMIN_SECRET, algorithms=["HS256"])
    except jwt.ExpiredSignatureError:
        raise HTTPException(status_code=401, detail="Token expired")
    except jwt.InvalidTokenError:
        raise HTTPException(status_code=401, detail="Invalid token")


# ═══════════════════════════════════════════════════════════
#  OPENAI-COMPATIBLE:  /v1/*
# ═══════════════════════════════════════════════════════════

@app.get("/v1/models")
async def list_models(authorization: str = Header(None)):
    """List available models (OpenAI-compatible)."""
    try:
        upstream_models = await ai_service.get_models()
    except Exception:
        upstream_models = ["dsv4", "mimo", "glm5", "minimaxm3"]

    tier = "free"
    if authorization:
        key = authorization.replace("Bearer ", "").strip()
        if key == Config.NINEROUTER_API_KEY:
            tier = "enterprise"
        else:
            async with async_session() as session:
                k_hash = _hash_key(key)
                result = await session.execute(
                    select(ApiKey).where(ApiKey.key_hash == k_hash, ApiKey.is_active == True)
                )
                ak = result.scalar_one_or_none()
                if ak:
                    tier = ak.tier

    allowed = Config.TIER_MODELS.get(tier, [])
    if "ALL" in allowed:
        filtered = upstream_models
    else:
        filtered = [m for m in upstream_models if m in allowed]

    return {
        "object": "list",
        "data": [{"id": m, "object": "model", "owned_by": "9router"} for m in filtered],
    }


class ChatMessage(BaseModel):
    role: str
    content: str


class ChatCompletionRequest(BaseModel):
    model: str = "dsv4"
    messages: List[ChatMessage]
    temperature: float = 0.7
    max_tokens: int = 2048
    stream: bool = False
    top_p: float = 1.0
    frequency_penalty: float = 0.0
    presence_penalty: float = 0.0


@app.post("/v1/chat/completions")
async def chat_completions(req: ChatCompletionRequest, authorization: str = Header(None)):
    """Chat completions — OpenAI-compatible proxy to 9Router."""
    api_key_obj, user, log_id = await _validate_api_key(authorization)

    tier = "enterprise"
    if api_key_obj:
        tier = api_key_obj.tier
    elif user:
        tier = user.tier

    # Check model access
    allowed_models = Config.TIER_MODELS.get(tier, [])
    if "ALL" not in allowed_models and req.model not in allowed_models:
        raise HTTPException(
            status_code=403,
            detail=f"Model '{req.model}' not available for tier '{tier}'. Upgrade your plan.",
        )

    try:
        response = await ai_service.chat(
            model=req.model,
            messages=[m.model_dump() for m in req.messages],
            temperature=req.temperature,
            max_tokens=req.max_tokens,
        )

        # Update usage log
        if log_id:
            async with async_session() as session:
                log = await session.get(UsageLog, log_id)
                if log:
                    log.model = req.model
                    usage = response.get("usage", {})
                    log.tokens_in = usage.get("prompt_tokens", 0)
                    log.tokens_out = usage.get("completion_tokens", 0)
                    await session.commit()

        return response

    except httpx.HTTPStatusError as e:
        raise HTTPException(status_code=e.response.status_code, detail=f"Upstream error: {e.response.text}")
    except Exception as e:
        raise HTTPException(status_code=502, detail=f"Gateway error: {str(e)}")


@app.get("/v1/dashboard/billing/usage")
async def billing_usage(authorization: str = Header(None)):
    """Usage stats for the authenticated API key."""
    api_key_obj, user, _ = await _validate_api_key(authorization)

    if api_key_obj is None:
        # Master key
        async with async_session() as session:
            result = await session.execute(
                select(func.sum(UsageLog.tokens_in), func.sum(UsageLog.tokens_out))
            )
            row = result.one()
            return {
                "object": "billing.usage",
                "total_prompt_tokens": row[0] or 0,
                "total_completion_tokens": row[1] or 0,
                "tier": "enterprise",
                "daily_limit": "unlimited",
            }

    async with async_session() as session:
        today = datetime.datetime.utcnow().replace(hour=0, minute=0, second=0, microsecond=0)
        result = await session.execute(
            select(func.sum(UsageLog.tokens_in), func.sum(UsageLog.tokens_out))
            .where(UsageLog.api_key_id == api_key_obj.id)
            .where(UsageLog.created_at >= today)
        )
        row = result.one()

    return {
        "object": "billing.usage",
        "total_prompt_tokens": row[0] or 0,
        "total_completion_tokens": row[1] or 0,
        "daily_requests_used": api_key_obj.daily_used,
        "daily_requests_limit": api_key_obj.daily_limit,
        "tier": api_key_obj.tier,
    }


# ═══════════════════════════════════════════════════════════
#  ADMIN DASHBOARD:  /api/*
# ═══════════════════════════════════════════════════════════

class LoginRequest(BaseModel):
    telegram_id: int


class ConfirmRequest(BaseModel):
    order_id: str


class TierUpdateRequest(BaseModel):
    tier: str


@app.post("/api/auth/login")
async def admin_login(req: LoginRequest):
    if req.telegram_id not in Config.ADMIN_IDS:
        raise HTTPException(status_code=403, detail="Not an admin")
    token = jwt.encode(
        {"telegram_id": req.telegram_id, "role": "admin", "exp": datetime.datetime.utcnow().timestamp() + 86400},
        ADMIN_SECRET,
        algorithm="HS256",
    )
    return {"token": token, "role": "admin"}


@app.get("/api/dashboard")
async def admin_dashboard(admin=Depends(_verify_admin_jwt)):
    stats = await user_service.get_stats()
    revenue = await payment_service.get_revenue_stats()
    return {**stats, **revenue}


@app.get("/api/users")
async def admin_users(page: int = 1, per_page: int = 50, admin=Depends(_verify_admin_jwt)):
    users, total = await user_service.get_all_users(page, per_page)
    return {
        "users": [
            {
                "id": u.id,
                "telegram_id": u.telegram_id,
                "username": u.username,
                "full_name": u.full_name,
                "tier": u.tier,
                "tier_expires_at": u.tier_expires_at.isoformat() if u.tier_expires_at else None,
                "daily_requests_used": u.daily_requests_used,
                "daily_requests_limit": u.daily_requests_limit,
                "total_requests": u.total_requests,
                "is_banned": u.is_banned,
                "created_at": u.created_at.isoformat() if u.created_at else None,
            }
            for u in users
        ],
        "total": total,
        "page": page,
        "per_page": per_page,
    }


@app.post("/api/users/{telegram_id}/ban")
async def admin_ban_user(telegram_id: int, admin=Depends(_verify_admin_jwt)):
    return {"success": await user_service.ban_user(telegram_id, True)}


@app.post("/api/users/{telegram_id}/unban")
async def admin_unban_user(telegram_id: int, admin=Depends(_verify_admin_jwt)):
    return {"success": await user_service.ban_user(telegram_id, False)}


@app.post("/api/users/{telegram_id}/tier")
async def admin_update_tier(telegram_id: int, req: TierUpdateRequest, admin=Depends(_verify_admin_jwt)):
    async with async_session() as session:
        result = await session.execute(select(User).where(User.telegram_id == telegram_id))
        user = result.scalar_one_or_none()
        if not user:
            raise HTTPException(status_code=404, detail="User not found")
        user.tier = req.tier
        user.daily_requests_limit = Config.TIER_LIMITS.get(req.tier, 20)
        await session.commit()
    return {"success": True, "tier": req.tier}


# === API KEYS ===

@app.get("/api/apikeys")
async def admin_list_apikeys(page: int = 1, per_page: int = 50, admin=Depends(_verify_admin_jwt)):
    async with async_session() as session:
        offset = (page - 1) * per_page
        result = await session.execute(
            select(ApiKey, User.username, User.telegram_id)
            .join(User, ApiKey.user_id == User.id)
            .order_by(ApiKey.created_at.desc())
            .offset(offset)
            .limit(per_page)
        )
        rows = result.all()
        count_result = await session.execute(select(func.count(ApiKey.id)))
        total = count_result.scalar()

    return {
        "api_keys": [
            {
                "id": ak.id,
                "user_id": ak.user_id,
                "username": username,
                "telegram_id": tg_id,
                "key_prefix": ak.key_prefix,
                "name": ak.name,
                "tier": ak.tier,
                "daily_used": ak.daily_used,
                "daily_limit": ak.daily_limit,
                "total_requests": ak.total_requests,
                "is_active": ak.is_active,
                "last_used_at": ak.last_used_at.isoformat() if ak.last_used_at else None,
                "created_at": ak.created_at.isoformat() if ak.created_at else None,
                "revoked_at": ak.revoked_at.isoformat() if ak.revoked_at else None,
            }
            for ak, username, tg_id in rows
        ],
        "total": total,
        "page": page,
    }


@app.post("/api/apikeys/{key_id}/revoke")
async def admin_revoke_apikey(key_id: int, admin=Depends(_verify_admin_jwt)):
    async with async_session() as session:
        key = await api_key_service.revoke(session, key_id)
        if not key:
            raise HTTPException(status_code=404, detail="API key not found")
        return {"success": True, "message": f"Key {key.key_prefix} revoked"}


@app.post("/api/apikeys/{key_id}/restore")
async def admin_restore_apikey(key_id: int, admin=Depends(_verify_admin_jwt)):
    async with async_session() as session:
        key = await api_key_service.restore(session, key_id)
        if not key:
            raise HTTPException(status_code=404, detail="API key not found")
        return {"success": True, "message": f"Key {key.key_prefix} restored"}


@app.delete("/api/apikeys/{key_id}")
async def admin_delete_apikey(key_id: int, admin=Depends(_verify_admin_jwt)):
    async with async_session() as session:
        deleted = await api_key_service.delete(session, key_id)
        if not deleted:
            raise HTTPException(status_code=404, detail="API key not found")
        return {"success": True, "message": "Key permanently deleted"}


# === TRANSACTIONS ===

@app.get("/api/transactions")
async def admin_transactions(page: int = 1, per_page: int = 50, admin=Depends(_verify_admin_jwt)):
    async with async_session() as session:
        offset = (page - 1) * per_page
        result = await session.execute(
            select(Transaction).order_by(Transaction.created_at.desc()).offset(offset).limit(per_page)
        )
        transactions = result.scalars().all()
        count_result = await session.execute(select(func.count(Transaction.id)))
        total = count_result.scalar()

    return {
        "transactions": [
            {
                "id": t.id,
                "user_id": t.user_id,
                "tier": t.tier,
                "amount": t.amount,
                "payment_status": t.payment_status,
                "external_id": t.external_id,
                "paid_at": t.paid_at.isoformat() if t.paid_at else None,
                "created_at": t.created_at.isoformat() if t.created_at else None,
            }
            for t in transactions
        ],
        "total": total,
        "page": page,
    }


@app.post("/api/transactions/{tx_id}/confirm")
async def admin_confirm_transaction(tx_id: int, admin=Depends(_verify_admin_jwt)):
    result = await payment_service.confirm_payment(tx_id)
    return result


# === STATS ===

@app.get("/api/stats/revenue")
async def admin_revenue_stats(admin=Depends(_verify_admin_jwt)):
    return await payment_service.get_revenue_stats()


@app.get("/api/stats/users")
async def admin_user_stats(admin=Depends(_verify_admin_jwt)):
    return await user_service.get_stats()


@app.get("/api/stats/usage")
async def admin_usage_stats(admin=Depends(_verify_admin_jwt)):
    async with async_session() as session:
        keys_count = await session.execute(select(func.count(ApiKey.id)))
        total_keys = keys_count.scalar()

        active_keys = await session.execute(
            select(func.count(ApiKey.id)).where(ApiKey.is_active == True)
        )
        active_count = active_keys.scalar()

        today = datetime.datetime.utcnow().replace(hour=0, minute=0, second=0, microsecond=0)
        today_req = await session.execute(
            select(func.count(UsageLog.id)).where(UsageLog.created_at >= today)
        )
        today_requests = today_req.scalar()

        tokens_result = await session.execute(
            select(func.sum(UsageLog.tokens_in), func.sum(UsageLog.tokens_out))
        )
        row = tokens_result.one()

    return {
        "total_api_keys": total_keys,
        "active_api_keys": active_count,
        "today_requests": today_requests,
        "total_prompt_tokens": row[0] or 0,
        "total_completion_tokens": row[1] or 0,
    }


# === ACTIONS ===

@app.post("/api/actions/reset-limits")
async def admin_reset_limits(admin=Depends(_verify_admin_jwt)):
    await user_service.reset_daily_limits()
    return {"success": True, "message": "Daily limits reset"}


# === HEALTH ===

@app.get("/api/health")
async def health_check():
    return {"status": "ok", "service": "ai-seller-api", "timestamp": datetime.datetime.utcnow().isoformat()}


@app.get("/")
async def root():
    return {
        "service": "AI Seller API",
        "version": "1.0.0",
        "endpoints": {
            "openai": "/v1/models, /v1/chat/completions, /v1/dashboard/billing/usage",
            "admin": "/api/auth/login, /api/dashboard, /api/users, /api/apikeys, /api/transactions",
            "health": "/api/health",
            "docs": "/docs",
        },
    }
