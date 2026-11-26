#!/usr/bin/env python3
"""Measure how well the similarity threshold separates relevant from unrelated documents.

Embeds the five bundled source documents exactly like the API does (900-character chunks,
200 overlap, "search_document: " prefix), then scores 22 hand-labelled fault queries
("search_query: <code> <description>"). For each threshold it reports how many relevant
documents are still found (recall) and how many queries leak an unrelated document.

Needs a local Ollama with the embedding model (default bge-m3). It does not touch the
database or the running stack. The query set is small and labelled by its author, so use
it to sanity-check a threshold, not to certify one.

Run: python3 scripts/eval_retrieval.py
Environment: OLLAMA_URL (default http://localhost:11434), OLLAMA_EMBED_MODEL (default bge-m3:latest)
"""
import importlib.util
import json
import math
import os
import urllib.request
from pathlib import Path
ROOT = Path(__file__).resolve().parents[1]
OLLAMA = os.environ.get('OLLAMA_URL', 'http://localhost:11434').rstrip('/')
MODEL = os.environ.get('OLLAMA_EMBED_MODEL', 'bge-m3:latest')
def embed(text):
    r=urllib.request.Request(OLLAMA+'/api/embed',data=json.dumps({'model':MODEL,'input':text}).encode(),headers={'Content-Type':'application/json'})
    return json.load(urllib.request.urlopen(r,timeout=300))['embeddings'][0]
def split(t):
    r=list(t.strip()); out=[]
    for s in range(0,len(r),700):
        e=min(s+900,len(r)); out.append(''.join(r[s:e]))
        if e==len(r): break
    return out
def cos(a,b):
    d=sum(x*y for x,y in zip(a,b)); return d/(math.sqrt(sum(x*x for x in a))*math.sqrt(sum(y*y for y in b)))
spec=importlib.util.spec_from_file_location('seed',ROOT/'scripts/seed_demo.py'); seed=importlib.util.module_from_spec(spec); spec.loader.exec_module(seed)
docs={'CRC':(ROOT/'docs/training/sources/ethernet-crc.md').read_text(),'RTO':(ROOT/'docs/training/sources/tcp-timeout.md').read_text(),'DHCP':(ROOT/'docs/training/sources/dhcp.md').read_text(),
      'LOOP':seed.DOCUMENTS[0]['content'],'COMM':seed.DOCUMENTS[1]['content']}
vecs={k:[embed('search_document: '+c) for c in split(v)] for k,v in docs.items()}
sc=json.load(open(ROOT/'docs/training/scenarios.json'))['cases']
NET={'CRC','RTO','DHCP','COMM'}
Q=[ # (kod, açıklama, ilgili, sınırda)
 (sc[0]['code'],sc[0]['description'],{'RTO'},NET-{'RTO'}),(sc[0]['code'],sc[0]['new_description'],{'RTO'},NET-{'RTO'}),
 (sc[1]['code'],sc[1]['description'],{'CRC'},NET-{'CRC'}),(sc[1]['code'],sc[1]['new_description'],{'CRC'},NET-{'CRC'}),
 (sc[2]['code'],sc[2]['description'],{'DHCP'},NET-{'DHCP'}),(sc[2]['code'],sc[2]['new_description'],{'DHCP'},NET-{'DHCP'}),
 ('CONNECTION_TIMEOUT','TCP bağlantısı kurulamıyor, istekler zaman aşımına düşüyor.',{'RTO'},NET-{'RTO'}),
 ('LINK_CRC_ERRORS','Switch portunda frame hataları ve paket kaybı gözlendi.',{'CRC'},NET-{'CRC'}),
 ('DHCP_ADDRESS_FAILURE','Cihaz IP adresi alamıyor, DHCP isteği yanıtsız kalıyor.',{'DHCP'},NET-{'DHCP'}),
 ('DETECTOR_LOOP_OPEN',seed.CASES[0]['events'][0][2],{'LOOP'},set()),
 ('DETECTOR_LOOP_OPEN',seed.CASES[1]['events'][0][2],{'LOOP'},set()),
 ('DETECTOR_LOOP_OPEN','Dedektör kanalı araç saymıyor, döngü kablosu kopuk olabilir.',{'LOOP'},set()),
 ('SIGNAL_COMM_LOSS',seed.CASES[2]['events'][0][2],{'COMM'},NET-{'COMM'}),
 ('SIGNAL_COMM_LOSS','Sinyal kontrolörü koordinasyon dışına düştü, ağ bağlantısı yok.',{'COMM'},NET-{'COMM'}),
 # ilgili belge OLMAMALI:
 ('UPS_BATTERY_LOW',seed.CASES[5]['events'][0][2],set(),set()),
 ('VMS_DISPLAY_FAULT',seed.MANUAL_REPORT['description'],set(),set()),
 ('CAMERA_STREAM_LOST',seed.CASES[3]['events'][0][2],set(),NET),
 ('CABINET_DOOR_OPEN','Saha kabini kapısı açık kalmış, kilit çalışmıyor.',set(),set()),
 ('CORROSION','Kabin gövdesinde paslanma ve boya atması var.',set(),set()),
 ('POWER_FAILURE','Kabinde enerji kesintisi oldu, cihaz kapandı.',set(),set()),
 ('SENSOR_CALIBRATION_DRIFT','Sıcaklık sensörü kalibrasyon kayması gösteriyor.',set(),set()),
 ('LAMP_OUT','Sinyal grubunda kırmızı lamba yanmıyor.',set(),set()),
]
rows=[]
for code,desc,rel,border in Q:
    q=embed('search_query: '+code+' '+desc)
    best={k:max(cos(q,v) for v in vs) for k,vs in vecs.items()}
    rows.append((code,rel,border,best))
print(f"{'sorgu':<27}{'beklenen':<8}  en iyi ilgili | en iyi ALAKASIZ (belge)")
for code,rel,border,best in rows:
    r=max((best[k] for k in rel),default=None)
    irr={k:v for k,v in best.items() if k not in rel and k not in border}
    ik=max(irr,key=irr.get); 
    print(f"{code:<27}{','.join(rel) or '-':<8}  {('%.3f'%r) if r is not None else '  -  ':>6}        | {irr[ik]:.3f} ({ik})")
print("\neşik  | ilgili belge bulunan (recall) | alakasız belge sızan sorgu (FP) / toplam")
nrel=sum(1 for r in rows if r[1]); nq=len(rows)
for t in [0.45,0.50,0.55,0.60,0.62,0.65,0.70,0.75]:
    hit=sum(1 for code,rel,border,best in rows if rel and max(best[k] for k in rel)>=t)
    fp=sum(1 for code,rel,border,best in rows if any(v>=t for k,v in best.items() if k not in rel and k not in border))
    print(f"{t:.2f}  | {hit}/{nrel}                         | {fp}/{nq}")
