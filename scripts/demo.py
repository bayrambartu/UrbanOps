#!/usr/bin/env python3
"""Create a virtual device and technician, then submit an idempotent demo event."""
import json
import os
from pathlib import Path
import secrets
from datetime import datetime, timezone
from urllib.request import Request, urlopen
from urllib.error import HTTPError
root = Path(__file__).resolve().parents[1]
config = dict(line.split('=', 1) for line in (root / '.env').read_text().splitlines() if line and not line.startswith('#'))
base = os.environ.get('API_URL', 'http://localhost:8080')
def request(path, token='', body=None):
    req = Request(base + '/api' + path, data=json.dumps(body).encode() if body is not None else None,
                  headers={'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token})
    try:
        with urlopen(req, timeout=15) as response:
            return json.load(response)
    except HTTPError as e:
        if e.code == 409:
            return None
        raise SystemExit(f'HTTP {e.code}: {e.read().decode()}')
session = request('/auth/login', body={'email': config['ADMIN_EMAIL'], 'password': config['ADMIN_PASSWORD']})
token = session['token']
request('/devices', token, {'id': 'TRF-104', 'name': 'Kadıköy Sinyal Kontrol Cihazı', 'kind': 'Trafik sinyalizasyon', 'location': 'Kadıköy'})
request('/users', token, {'email': 'teknisyen@urbanops.local', 'name': 'Demo Teknisyen', 'role': 'technician', 'password': secrets.token_urlsafe(24)})
event = {'event_id': 'demo-' + secrets.token_hex(12), 'device_id': 'TRF-104', 'fault_code': 'CONNECTION_TIMEOUT', 'severity': 'high', 'location': 'Kadıköy', 'description': 'Simülasyon: cihaz bağlantısı kesildi', 'timestamp': datetime.now(timezone.utc).isoformat()}
request('/events', config['INGEST_TOKEN'], event)
request('/events', config['INGEST_TOKEN'], event)
print('Cihaz, demo teknisyen ve olay hazır. Aynı olay iki kez gönderildi; tek olay olarak işlenmeli.')
