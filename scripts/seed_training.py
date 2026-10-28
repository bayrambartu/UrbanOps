#!/usr/bin/env python3
"""Populate onboarding scenarios through public APIs, preserving real records.
Run: python3 scripts/seed_training.py [--skip-documents]
"""
import argparse
from datetime import datetime, timezone, timedelta
import json
import os
from pathlib import Path
import secrets
import time
from urllib.error import HTTPError
from urllib.request import Request, urlopen

ROOT = Path(__file__).resolve().parents[1]
DATA = ROOT / 'docs/training'
LOCAL = ROOT / '.local'

class API:
    def __init__(self, base):
        self.base = base.rstrip('/')
    def call(self, path, token='', body=None, method=None):
        request = Request(self.base + '/api' + path,
            data=json.dumps(body, ensure_ascii=False).encode() if body is not None else None,
            method=method or ('POST' if body is not None else 'GET'),
            headers={'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token})
        try:
            with urlopen(request, timeout=150) as response:
                return json.load(response)
        except HTTPError as error:
            detail = error.read().decode(errors='replace')
            raise RuntimeError(f'{path}: HTTP {error.code}: {detail}') from error
    def login(self, account):
        return self.call('/auth/login', body={'email': account['email'], 'password': account['password']})['token']

def private_json(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n')
    path.chmod(0o600)

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--skip-documents', action='store_true')
    args = parser.parse_args()
    config = dict(line.split('=', 1) for line in (ROOT / '.env').read_text().splitlines() if line and not line.startswith('#'))
    plan = json.loads((DATA / 'scenarios.json').read_text())
    LOCAL.mkdir(exist_ok=True, mode=0o700)
    accounts_path = LOCAL / 'training-accounts.json'
    if accounts_path.exists():
        accounts = json.loads(accounts_path.read_text())
    else:
        accounts = [dict(person, password=secrets.token_urlsafe(18)) for person in plan['people']]
        private_json(accounts_path, accounts)
    by_email = {a['email']: a for a in accounts}
    api = API(os.environ.get('API_URL', 'http://localhost:8080'))
    admin = api.login({'email': config['ADMIN_EMAIL'], 'password': config['ADMIN_PASSWORD']})
    existing = {u['email']: u for u in api.call('/users', admin)}
    for person in plan['people']:
        email = person['email']
        if email not in existing:
            api.call('/users', admin, by_email[email])
        elif existing[email]['role'] != person['role']:
            raise RuntimeError(f'{email}: mevcut rol farklı; hesap değiştirilmedi')
    users = {u['email']: u for u in api.call('/users', admin)}
    # At most ten logins (bootstrap + nine people) per run, within the API limit.
    tokens = {email: api.login(by_email[email]) for email in by_email}
    print('Personel hazır: 3 yönetici, 3 operatör, 3 teknisyen.', flush=True)
    devices = {d['id'] for d in api.call('/devices', admin)}
    for case in plan['cases']:
        device = case['device']
        if device['id'] not in devices:
            api.call('/devices', tokens[case['operator']], device)

    def order_for(case, phase):
        marker = f"[{plan['version']}/{phase}]"
        deadline = time.monotonic() + 45
        while True:
            faults = api.call('/faults', admin)
            fault = next((f for f in faults if f['device_id'] == case['device']['id'] and marker in f['description']), None)
            if fault:
                order = next((w for w in api.call('/work-orders', admin) if w['fault_id'] == fault['id']), None)
                if order:
                    return order
            if time.monotonic() > deadline:
                raise RuntimeError(f"{case['device']['id']}: olay işlenmedi; worker ve DLQ kontrol edilmeli")
            time.sleep(1)

    def event(case, phase):
        timestamp = datetime.now(timezone.utc) - (timedelta(days=7) if phase == 'history' else timedelta())
        api.call('/events', config['INGEST_TOKEN'], {
            'event_id': f"{plan['version']}-{phase}-{case['device']['id']}",
            'device_id': case['device']['id'], 'fault_code': case['code'], 'severity': case['severity'],
            'location': case['device']['location'],
            'description': f"[{plan['version']}/{phase}] " + case['description' if phase == 'history' else 'new_description'],
            'timestamp': timestamp.isoformat()})
        return order_for(case, phase)

    for case in plan['cases']:
        order = event(case, 'history')
        path = '/work-orders/' + str(order['id'])
        expert = users[case['expert']]['id']
        if order['status'] == 'open':
            api.call(path, tokens[case['manager']], {'status': 'assigned', 'assignee_id': expert, 'note': case['assignment']}, 'PATCH')
            order = order_for(case, 'history')
        if order['status'] != 'closed' and order['assignee_id'] != expert:
            raise RuntimeError('Demo geçmiş işinin sorumlusu değişmiş; üzerine yazılmadı')
        if order['status'] == 'assigned':
            api.call(path, tokens[case['expert']], {'status': 'in_progress', 'note': 'EĞİTİM: kanıt ve sayaç incelemesi başlatıldı.'}, 'PATCH')
            order = order_for(case, 'history')
        if order['status'] == 'in_progress':
            api.call(path, tokens[case['expert']], {'status': 'resolved', 'note': case['resolution']}, 'PATCH')
            order = order_for(case, 'history')
        if order['status'] == 'resolved':
            api.call(path, tokens[case['manager']], {'status': 'closed', 'note': case['approval']}, 'PATCH')
            order = order_for(case, 'history')
        if order['status'] != 'closed':
            raise RuntimeError('Demo geçmiş işi beklenmeyen durumda; değiştirilmedi')
        print(f"Onaylı eğitim geçmişi: {case['code']} / İE-{order['id']}", flush=True)

    newbie = 'ali.yildiz@urbanops.local'
    assignments = []
    for case in plan['cases']:
        order = event(case, 'current')
        if order['status'] == 'open':
            api.call('/work-orders/' + str(order['id']), tokens[case['manager']],
                {'status': 'assigned', 'assignee_id': users[newbie]['id'], 'note': case['new_assignment']}, 'PATCH')
            order = order_for(case, 'current')
        assignments.append({'code': case['code'], 'fault_id': order['fault_id'], 'work_order_id': order['id'], 'assignee_id': order['assignee_id'], 'status': order['status']})
    private_json(LOCAL / 'training-results.json', {'new_technician': newbie, 'assignments': assignments})
    print('Yeni personele 3 benzer iş hazırlandı; mevcut eğitim ilerlemesine dokunulmadı.', flush=True)

    failures = []
    if not args.skip_documents:
        existing_docs = {d['title'] for d in api.call('/documents', admin)}
        for document in plan['documents']:
            if document['title'] in existing_docs:
                continue
            try:
                result = api.call('/documents', admin, {'title': document['title'], 'content': (DATA / document['file']).read_text()})
                state = "indeksli" if result.get("status") == "indexed" else "metin kayıtlı; vektör indeksi bekliyor"
                print(f"Kaynak hazır ({state}): {document['title']} / #{result['id']}", flush=True)
            except RuntimeError as error:
                failures.append(str(error))
    print('Hesap bilgileri: .local/training-accounts.json', flush=True)
    if failures:
        raise RuntimeError('Personel ve işler oluşturuldu; belge indeksleme eksik:\n' + '\n'.join(failures))

if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        raise SystemExit(str(error))
