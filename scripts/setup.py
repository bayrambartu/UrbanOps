#!/usr/bin/env python3
"""Create local credentials without overwriting an existing environment."""
from pathlib import Path
import secrets
root = Path(__file__).resolve().parents[1]
target = root / '.env'
if target.exists():
    raise SystemExit('.env zaten mevcut; üzerine yazılmadı.')
values = {
    'POSTGRES_PASSWORD': secrets.token_hex(20),
    'JWT_SECRET': secrets.token_hex(32),
    'INGEST_TOKEN': secrets.token_hex(32),
    'ADMIN_EMAIL': 'admin@urbanops.local',
    'ADMIN_PASSWORD': secrets.token_urlsafe(24),
    'OLLAMA_CHAT_MODEL': 'qwen2.5:7b',
    'OLLAMA_EMBED_MODEL': 'nomic-embed-text',
}
with target.open('x') as f:
    f.write(''.join(f'{k}={v}\n' for k, v in values.items()))
target.chmod(0o600)
print('.env oluşturuldu. Giriş bilgilerini bu dosyadan okuyabilirsiniz.')
