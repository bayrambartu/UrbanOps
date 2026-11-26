#!/usr/bin/env python3
"""Populate a presentation-ready, fully synthetic demo data set through the public API.

Everything here is invented: people, devices, fault descriptions, resolutions and the
two knowledge documents. Nothing is measured on real infrastructure.

The data set tells one story across seven work orders, each left in a different state so
that every screen of the panel has something meaningful to show:

  * a closed, manager-approved loop-detector repair (the "history"),
  * a new fault with the same code on another device, assigned to a new technician,
    so the assistant can cite the approved solution,
  * a critical fault reported three times by the device (one fault, three occurrences),
  * a job on hold, a job awaiting manager approval, a job with a transfer request,
  * a fault nobody has been assigned to, and a fault code with no history at all.

Safe to re-run: devices, users, events and documents are matched by id/e-mail/title and
workflow steps only run when the work order has not reached that state yet. It never
overwrites a work order that someone else has changed. It only adds data; for a perfectly
clean screenshot environment start from an empty stack first (this deletes ALL data):

    docker compose down -v && docker compose up --build -d api worker

Run:  python3 scripts/seed_demo.py [--skip-documents]
Environment overrides: API_URL, ADMIN_EMAIL, ADMIN_PASSWORD, INGEST_TOKEN, DEMO_ACCOUNTS_FILE
Demo account passwords are random and kept in .local/demo-accounts.json (git-ignored).
The API allows 10 logins per minute per client; a run uses 5, so wait a minute between runs.
"""
import argparse
from datetime import datetime, timedelta, timezone
import json
import os
from pathlib import Path
import secrets
import time
from urllib.error import HTTPError
from urllib.request import Request, urlopen

ROOT = Path(__file__).resolve().parents[1]
VERSION = 'demo-v1'

PEOPLE = [
    {'name': 'Selin Arslan', 'email': 'selin.arslan@urbanops.local', 'role': 'operator'},
    {'name': 'Murat Demir', 'email': 'murat.demir@urbanops.local', 'role': 'technician'},
    {'name': 'Elif Kaya', 'email': 'elif.kaya@urbanops.local', 'role': 'technician'},
    {'name': 'Can Yurt', 'email': 'can.yurt@urbanops.local', 'role': 'technician'},
]
MANAGER = 'selin.arslan@urbanops.local'

DEVICES = [
    {'id': 'SIG-KDK-014', 'name': 'Kadıköy Kavşağı Sinyal Kontrolörü', 'kind': 'Trafik sinyalizasyon kontrolörü', 'location': 'Kadıköy'},
    {'id': 'DET-BSK-007', 'name': 'Beşiktaş Kavşağı Araç Dedektörü (Loop)', 'kind': 'Araç dedektörü', 'location': 'Beşiktaş'},
    {'id': 'DET-SIS-012', 'name': 'Şişli Kavşağı Araç Dedektörü (Loop)', 'kind': 'Araç dedektörü', 'location': 'Şişli'},
    {'id': 'VMS-D100-03', 'name': 'D-100 Değişken Mesaj Panosu', 'kind': 'Değişken mesaj panosu', 'location': 'Ataşehir'},
    {'id': 'CAM-USK-021', 'name': 'Üsküdar Meydan Trafik Kamerası', 'kind': 'Trafik kamerası', 'location': 'Üsküdar'},
    {'id': 'SW-BGC-005', 'name': 'Bağcılar Saha Kabini Ağ Anahtarı', 'kind': 'Ağ anahtarı', 'location': 'Bağcılar'},
    {'id': 'UPS-ZTB-002', 'name': 'Zeytinburnu Saha Kabini UPS', 'kind': 'Enerji / UPS', 'location': 'Zeytinburnu'},
]

H = timedelta(hours=1)
MIN = timedelta(minutes=1)
DAY = timedelta(days=1)

# end: the state the work order is left in. tech: who is responsible.
CASES = [
    {   # 1. History: closed and approved, the source of the assistant's answer in case 2.
        'device': 'DET-BSK-007', 'code': 'DETECTOR_LOOP_OPEN', 'tech': 'murat.demir@urbanops.local', 'end': 'closed',
        'events': [(-9 * DAY, 'high', 'Kanal 3 araç algılamıyor; kontrolör döngü açık devre uyarısı veriyor.')],
        'assign_note': 'Hafta sonu yol çalışması sonrası başladı; ilk müdahale sizde.',
        'start_note': 'Saha ziyareti: döngü direnci ve izolasyon ölçümü başlatıldı.',
        'resolve_note': ('Demo çözüm kaydı. Döngü kablosunun kesim kanalı girişinde zedelendiği ve sürekliliğin kesildiği '
                         'ölçüldü. Zedelenen bölüm kesilip yeni döngü sarıldı, dedektör kartında hassasiyet yeniden '
                         'ayarlandı. 30 dakikalık izlemede araç sayımı normale döndü.'),
        'close_note': 'Ölçüm değerleri ve izleme sonucu kontrol edildi, çözüm onaylandı.',
    },
    {   # 2. Same code on another device: the assistant should cite case 1.
        'device': 'DET-SIS-012', 'code': 'DETECTOR_LOOP_OPEN', 'tech': 'can.yurt@urbanops.local', 'end': 'assigned',
        'events': [(-40 * MIN, 'medium', 'Kanal 1 sayım üretmiyor; döngü açık devre uyarısı alındı.')],
        'assign_note': 'Benzer bir arıza geçen hafta çözüldü; geçmiş çözümü inceleyip işe başlayın.',
    },
    {   # 3. One fault, three device reports, severity rising to critical.
        'device': 'SIG-KDK-014', 'code': 'SIGNAL_COMM_LOSS', 'tech': 'elif.kaya@urbanops.local', 'end': 'in_progress',
        'events': [
            (-3 * H, 'medium', 'Merkezle haberleşme kesildi; kontrolör yerel sabit planda çalışıyor.'),
            (-2 * H, 'high', 'Haberleşme hâlâ kurulamadı; koordinasyon dışı çalışma sürüyor.'),
            (-25 * MIN, 'critical', 'Üçüncü kopma: yoğun saatte koordinasyon dışı çalışma, kuyruk uzuyor.'),
        ],
        'assign_note': 'Kritik: yoğun saatte koordinasyon kaybı var, öncelikli ilgilenin.',
        'start_note': 'Sahaya çıkıldı, kabin güç ve bağlantı ışıkları kontrol ediliyor.',
    },
    {   # 4. On hold, waiting for a part.
        'device': 'CAM-USK-021', 'code': 'CAMERA_STREAM_LOST', 'tech': 'murat.demir@urbanops.local', 'end': 'on_hold',
        'events': [(-5 * H, 'high', 'Kamera görüntü akışı kesildi; cihaz ağda ping almıyor.')],
        'assign_note': 'Üsküdar bölgesi sizde.',
        'start_note': 'Direk dibi kabinde kontrol yapıldı, ağ tarafı sağlam görünüyor.',
        'hold_note': 'Kameranın PoE enjektörü arızalı görünüyor; yedek enjektör teslimi bekleniyor.',
    },
    {   # 5. Solved by the technician, waiting for the manager's approval.
        'device': 'SW-BGC-005', 'code': 'LINK_CRC_ERRORS', 'tech': 'elif.kaya@urbanops.local', 'end': 'resolved',
        'events': [(-26 * H, 'medium', 'Yukarı bağlantı portunda artan CRC hata sayacı gözlendi.')],
        'assign_note': 'Bağcılar kabini, port hataları artıyor.',
        'start_note': 'Port sayaçları kaydedildi, kablo ve konnektör incelemesine başlandı.',
        'resolve_note': ('Demo çözüm kaydı. Patch kablosunun konnektörünün gevşediği görüldü; kablo yenilendi, '
                         'port sayaçları sıfırlandı. 2 saat sonra yeni CRC hatası oluşmadığı doğrulandı.'),
    },
    {   # 6. A code with no history: the assistant must not invent an answer. Transfer requested.
        'device': 'UPS-ZTB-002', 'code': 'UPS_BATTERY_LOW', 'tech': 'can.yurt@urbanops.local', 'end': 'in_progress',
        'transfer': 'Bu hafta başka bir bölgede planlı çalışmam var; yedek akü ölçümü için ekipman ve zaman ayıramıyorum.',
        'events': [(-7 * H, 'medium', 'UPS batarya kapasitesi düşük uyarısı verdi; şebeke kesintisinde kısa süre dayanıyor.')],
        'assign_note': 'Zeytinburnu kabini UPS uyarısı.',
        'start_note': 'Batarya gerilimleri ölçülüyor.',
    },
]

# Reported by hand from the field instead of by the device itself.
MANUAL_REPORT = {
    'by': 'elif.kaya@urbanops.local', 'request_id': VERSION + '-vms-d100-03', 'device_id': 'VMS-D100-03',
    'fault_code': 'VMS_DISPLAY_FAULT', 'severity': 'medium', 'observed_ago': 90 * MIN,
    'description': 'Panonun alt iki satırında piksel grupları yanmıyor; mesaj okunaksız görünüyor.',
    'location_detail': 'D-100 Ataşehir yönü, 2. portal',
    'field_note': 'Pano kapağı kapalı; dış hasar görünmüyor. Besleme modülü ilk şüpheli.',
}

DOCUMENTS = [
    {'title': 'Demo kaynağı · Araç dedektörü döngüsü (loop) arıza kontrol listesi',
     'content': ('DETECTOR_LOOP_OPEN: Araç dedektörü döngüsünde açık devre uyarısı. Bu metin gösterim amaçlı genel bir '
                 'kontrol listesidir; üretici talimatı veya saha prosedürü değildir.\n\n'
                 '1. Kontrolör veya dedektör kartında ilgili kanalın uyarısını ve zamanını kaydedin.\n'
                 '2. Döngü kablosunun besleme hattı ve bağlantı kutusundaki eklemleri gözle inceleyin; gevşek, '
                 'oksitlenmiş veya nemli bağlantıları not edin.\n'
                 '3. Döngünün direncini ve toprağa göre yalıtımını uygun ölçü aletiyle ölçün; açık devre veya '
                 'yalıtım kaybı varsa ölçümü kayda ekleyin.\n'
                 '4. Hasar yol kesim kanalı çevresindeyse yakın zamanda yol çalışması yapılıp yapılmadığını '
                 'araştırın.\n'
                 '5. Onarımdan sonra dedektör hassasiyetini kontrol edin ve araç sayımının normale döndüğünü '
                 'izleyerek doğrulayın. Ölçüm değerlerini çözüm notuna yazın.\n')},
    {'title': 'Demo kaynağı · Sinyal kontrolörü haberleşme kaybı kontrol listesi',
     'content': ('SIGNAL_COMM_LOSS: Sinyal kontrolörünün merkezle haberleşmesi kesildi. Bu metin gösterim amaçlı '
                 'genel bir kontrol listesidir; üretici talimatı veya saha prosedürü değildir.\n\n'
                 '1. Kontrolörün yerel planda çalışıp çalışmadığını ve koordinasyon dışı kaldığı süreyi kaydedin.\n'
                 '2. Kabin güç beslemesini ve ağ cihazının bağlantı ışıklarını kontrol edin.\n'
                 '3. Kontrolör ile ağ anahtarı arasındaki kabloyu ve konnektörleri inceleyin; gerekirse '
                 'geçici olarak yenisiyle değiştirip bağlantıyı deneyin.\n'
                 '4. Aynı hatta diğer cihazlarda da kopma olup olmadığına bakın; yaygın kopma hat veya merkez '
                 'tarafına işaret eder.\n'
                 '5. Haberleşme geri geldikten sonra zaman senkronizasyonunu ve koordinasyon planına dönüşü '
                 'doğrulayın.\n')},
]


def load_env():
    values = {}
    path = ROOT / '.env'
    if path.exists():
        values = dict(line.split('=', 1) for line in path.read_text().splitlines() if line and not line.startswith('#') and '=' in line)
    return lambda key: os.environ.get(key) or values.get(key) or ''


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
            raise RuntimeError(f'{path}: HTTP {error.code}: {error.read().decode(errors="replace")}') from error

    def login(self, email, password):
        return self.call('/auth/login', body={'email': email, 'password': password})['token']


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--skip-documents', action='store_true')
    args = parser.parse_args()
    env = load_env()
    for key in ('ADMIN_EMAIL', 'ADMIN_PASSWORD', 'INGEST_TOKEN'):
        if not env(key):
            raise SystemExit(f'{key} bulunamadı (.env veya ortam değişkeni).')
    api = API(os.environ.get('API_URL', 'http://localhost:8080'))

    accounts_path = Path(os.environ.get('DEMO_ACCOUNTS_FILE') or ROOT / '.local' / 'demo-accounts.json')
    accounts_path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    accounts = json.loads(accounts_path.read_text()) if accounts_path.exists() else {}
    for person in PEOPLE:
        accounts.setdefault(person['email'], secrets.token_urlsafe(18))
    accounts_path.write_text(json.dumps(accounts, ensure_ascii=False, indent=2) + '\n')
    accounts_path.chmod(0o600)

    admin = api.login(env('ADMIN_EMAIL'), env('ADMIN_PASSWORD'))
    existing = {u['email']: u for u in api.call('/users', admin)}
    for person in PEOPLE:
        email = person['email']
        if email in existing:
            if existing[email]['role'] != person['role']:
                raise SystemExit(f'{email}: mevcut rol farklı; hesap değiştirilmedi.')
            continue
        api.call('/users', admin, dict(person, password=accounts[email]))
    users = {u['email']: u for u in api.call('/users', admin)}
    # Five logins in total (admin + four people), well inside the API's per-minute limit.
    tokens = {}
    for person in PEOPLE:
        try:
            tokens[person['email']] = api.login(person['email'], accounts[person['email']])
        except RuntimeError as error:
            raise SystemExit(f"{person['email']}: giriş başarısız ({error}). Hesap daha önce başka bir parolayla "
                             'oluşturulmuş olabilir; mevcut hesabı silmeden bu betik düzeltemez.')
    print('Personel hazır: 1 operatör, 3 teknisyen.', flush=True)

    manager = tokens[MANAGER]
    known = {d['id'] for d in api.call('/devices', admin)}
    for device in DEVICES:
        if device['id'] not in known:
            api.call('/devices', manager, device)
    print(f'Cihaz envanteri hazır: {len(DEVICES)} cihaz.', flush=True)

    now = datetime.now(timezone.utc)

    def find(device, code, occurrences):
        deadline = time.monotonic() + 60
        while True:
            faults = [f for f in api.call('/faults', admin) if f['device_id'] == device and f['fault_code'] == code]
            if faults:
                fault = max(faults, key=lambda f: f['id'])
                if int(fault['occurrences']) >= occurrences:
                    order = next(w for w in api.call('/work-orders', admin) if w['fault_id'] == fault['id'])
                    return fault, order
            if time.monotonic() > deadline:
                raise SystemExit(f'{device}/{code}: olay işlenmedi; worker ve DLQ kontrol edilmeli.')
            time.sleep(1)

    summary = []
    for case in CASES:
        for index, (offset, severity, description) in enumerate(case['events'], start=1):
            api.call('/events', env('INGEST_TOKEN'), {
                'event_id': f"{VERSION}-{case['device']}-{index}", 'device_id': case['device'],
                'fault_code': case['code'], 'severity': severity,
                'location': next(d['location'] for d in DEVICES if d['id'] == case['device']),
                'description': description, 'timestamp': (now + offset).isoformat()})
        fault, order = find(case['device'], case['code'], len(case['events']))
        order = advance(api, admin, case, order, users, tokens, manager)
        summary.append((case['device'], case['code'], order))

    report = MANUAL_REPORT
    # The API rejects the same request key with different content (the observation time moves on
    # every run), so only send the report when it is not on record yet.
    sent = api.call('/fault-reports', tokens[report['by']])
    if not any(r['device_id'] == report['device_id'] and r['fault_code'] == report['fault_code'] for r in sent):
        api.call('/fault-reports', tokens[report['by']], {
            'request_id': report['request_id'], 'device_id': report['device_id'], 'fault_code': report['fault_code'],
            'severity': report['severity'], 'description': report['description'],
            'observed_at': (now - report['observed_ago']).isoformat(),
            'location_detail': report['location_detail'], 'field_note': report['field_note']})
    fault, order = find(report['device_id'], report['fault_code'], 1)
    summary.append((report['device_id'], report['fault_code'], order))

    for device, code, order in summary:
        print(f"  İE-{order['id']:<3} {device:<12} {code:<22} {order['status']}", flush=True)

    if not args.skip_documents:
        titles = {d['title'] for d in api.call('/documents', admin)}
        for document in DOCUMENTS:
            if document['title'] in titles:
                continue
            result = api.call('/documents', admin, document)
            state = 'indeksli' if result.get('status') == 'indexed' else 'metin kayıtlı; vektör indeksi bekliyor'
            print(f"Kaynak hazır ({state}): {document['title']}", flush=True)
    print(f'Demo verisi hazır. Hesap parolaları: {accounts_path}', flush=True)


def advance(api, admin, case, order, users, tokens, manager):
    """Move one work order forward to case['end'] without ever overwriting someone else's work."""
    oid = order['id']
    path = f'/work-orders/{oid}'
    tech_email = case['tech']
    tech_id = users[tech_email]['id']
    target = case['end']

    def current():
        return next(w for w in api.call('/work-orders', admin) if w['id'] == oid)

    def patch(token, body):
        api.call(path, token, body, 'PATCH')
        return current()

    order = current()
    if order['assignee_id'] not in (None, tech_id):
        raise SystemExit(f"İE-{oid}: sorumlu değişmiş; demo verisi üzerine yazılmadı.")
    if order['status'] == 'open' and target != 'open':
        order = patch(manager, {'status': 'assigned', 'assignee_id': tech_id, 'note': case['assign_note']})
    if order['status'] == 'assigned' and target in ('in_progress', 'on_hold', 'resolved', 'closed'):
        order = patch(tokens[tech_email], {'status': 'in_progress', 'note': case['start_note']})
    if order['status'] == 'in_progress' and target == 'on_hold':
        order = patch(tokens[tech_email], {'status': 'on_hold', 'note': case['hold_note']})
    if order['status'] == 'in_progress' and target in ('resolved', 'closed'):
        order = patch(tokens[tech_email], {'status': 'resolved', 'note': case['resolve_note']})
    if order['status'] == 'resolved' and target == 'closed':
        order = patch(manager, {'status': 'closed', 'note': case['close_note']})
    if case.get('transfer') and order['status'] in ('assigned', 'in_progress', 'on_hold') and not order['reassignment_requested']:
        order = patch(tokens[tech_email], {'action': 'request_reassignment', 'note': case['transfer']})
    return order


if __name__ == '__main__':
    main()
