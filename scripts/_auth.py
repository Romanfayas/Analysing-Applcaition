"""Shared sign-in for the verification scripts.

Signs in through the real frontend flow (/api/auth/*). Set E2E_EMAIL and E2E_PASSWORD to reuse an
existing account; otherwise a throwaway account (e2e-<random>@example.test) is registered, which only
works while self-service registration is enabled (the development default).
"""
import json
import os
import re
import secrets
import urllib.error
import urllib.request

FRONTEND = os.environ.get("E2E_FRONTEND", "http://localhost:3000")
COOKIE_NAME = "he_session"


def _post(path, body):
    req = urllib.request.Request(
        FRONTEND + path, data=json.dumps(body).encode(), headers={"Content-Type": "application/json"}, method="POST"
    )
    try:
        r = urllib.request.urlopen(req, timeout=30)
        return r.status, r.headers.get("Set-Cookie", "")
    except urllib.error.HTTPError as e:
        return e.code, ""


def sign_in() -> str:
    """Returns the session JWT (the value of the httpOnly session cookie)."""
    email, password = os.environ.get("E2E_EMAIL"), os.environ.get("E2E_PASSWORD")
    if email and password:
        status, cookie = _post("/api/auth/login", {"email": email, "password": password})
    else:
        email, password = f"e2e-{secrets.token_hex(4)}@example.test", secrets.token_urlsafe(16)
        status, cookie = _post("/api/auth/register", {"email": email, "password": password, "name": "E2E Script"})
    m = re.search(rf"{COOKIE_NAME}=([^;]+)", cookie)
    if status not in (200, 201) or not m:
        raise SystemExit(f"sign-in failed (HTTP {status}); set E2E_EMAIL/E2E_PASSWORD or enable registration")
    return m.group(1)
