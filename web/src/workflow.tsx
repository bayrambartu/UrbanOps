import { useState } from 'react';
type Row = Record<string, any>;
type Change = { status?: string; action?: string; assignee_id?: number; note?: string };
const names: Record<string, string> = { open: 'Açık', assigned: 'Atandı', in_progress: 'İşlemde', on_hold: 'Beklemede', resolved: 'Çözüm onayında', closed: 'Kapatıldı' };
export function TechnicianWorkload({ technicians }: { technicians: Row[] }) {
  return <section className="panel"><div className="panel-heading"><h2>Teknisyen iş yükü</h2><span className="muted">Atama için güncel iş dağılımı</span></div>
    <div className="workload-grid">{technicians.map(t => <article className="workload-card" key={t.id}><strong>{t.name}</strong><small>{Number(t.in_progress_count) ? 'Aktif müdahalesi var' : 'Aktif müdahalesi yok'}</small><div className="workload-counts"><span>Atandı <b>{t.assigned_count}</b></span><span>İşlemde <b>{t.in_progress_count}</b></span><span>Beklemede <b>{t.on_hold_count}</b></span></div>{Number(t.transfer_requests) > 0 && <p className="transfer-flag">{t.transfer_requests} devir talebi</p>}</article>)}</div>
    {!technicians.length && <p className="muted">Atama yapabilmek için yöneticinin Kullanıcılar ekranından teknisyen hesabı oluşturması gerekir.</p>}
  </section>;
}
export function WorkOrders({ orders, busy, onInspect, personal }: { orders: Row[]; busy: boolean; onInspect: (o: Row) => void; personal: boolean }) {
  const [status, setStatus] = useState('active');
  const active = ['assigned', 'in_progress', 'on_hold'];
  const visible = orders.filter(o => status === 'all' || (status === 'transfers' ? o.reassignment_requested && active.includes(o.status) : status === 'active' ? !['resolved', 'closed'].includes(o.status) : o.status === status));
  return <section className="panel"><div className="panel-heading"><h2>{personal ? 'Bana atanan işler' : 'Bakım iş akışı'}</h2><select className="workflow-filter" aria-label="İş emri durumu" value={status} onChange={e => setStatus(e.target.value)}><option value="active">Aktif işler</option><option value="all">Tüm işler</option>{!personal&&<option value="transfers">Devir talepleri</option>}{Object.entries(names).map(([v, label]) => <option key={v} value={v}>{label}</option>)}</select></div>
    <p className="muted">{personal ? 'İşler öncelik ve bekleme süresine göre sıralanır. Müsait olduğunuzda işe başlayın; engel varsa gerekçesiyle beklemeye alın.' : 'Atama → Teknisyen müdahalesi → Çözüm kontrolü → Kapatma'}</p>
    <div className="workflow-tabs">{Object.entries(names).map(([value,label])=><button key={value} className={status===value?'active':'secondary'} onClick={()=>setStatus(value)}>{label} <b>{orders.filter(o=>o.status===value).length}</b></button>)}{!personal&&<button className={status==='transfers'?'active':'secondary'} onClick={()=>setStatus('transfers')}>Devir talepleri <b>{orders.filter(o=>o.reassignment_requested&&active.includes(o.status)).length}</b></button>}</div>
    <div className="table-wrap"><table><thead><tr><th>İŞ EMRİ</th><th>CİHAZ / KONUM</th><th>PERSONEL</th><th>DURUM</th><th>OLUŞTURULMA</th><th /></tr></thead><tbody>{visible.map(o => <tr key={o.id}><td><strong>İE-{o.id} · {o.severity === 'critical' ? 'Kritik' : o.severity === 'high' ? 'Yüksek' : o.severity === 'medium' ? 'Orta' : 'Düşük'}</strong><small>{o.fault_code}</small></td><td>{o.device_id}<small>{o.location}</small></td><td>{o.assignee || 'Atama bekliyor'}</td><td><span className={'badge ' + o.status}>{names[o.status]}</span>{o.hold_reason && <small className="hold-note">{o.hold_reason}</small>}{o.reassignment_requested && active.includes(o.status) && <small className="transfer-flag">Devir talebi: {o.reassignment_reason}</small>}</td><td>{new Date(o.created_at).toLocaleString('tr-TR', { dateStyle: 'short', timeStyle: 'short' })}</td><td><button className="text" disabled={busy} onClick={() => onInspect(o)}>{personal ? 'İşi aç ↗' : 'Yönet ↗'}</button></td></tr>)}</tbody></table></div>
    {!visible.length && <div className="empty"><p>{personal ? 'Bu durumda size atanmış bir iş bulunmuyor.' : 'Bu durumda iş emri bulunmuyor.'}</p></div>}
  </section>;
}
export function WorkOrderActions({ order, manager, userID, technicians, busy, onChange }: { order: Row; manager: boolean; userID: number; technicians: Row[]; busy: boolean; onChange: (payload: Change) => Promise<void> }) {
  const owner = !manager && Number(order.assignee_id) === userID;
  const active = ['assigned', 'in_progress', 'on_hold'].includes(order.status);
  const submit = (e: React.FormEvent<HTMLFormElement>, payload: Change) => {
    e.preventDefault(); const data = new FormData(e.currentTarget);
    if (data.has('note')) payload.note = String(data.get('note'));
    if (data.has('assignee_id')) payload.assignee_id = Number(data.get('assignee_id'));
    void onChange(payload);
  };
  const options = technicians.filter(t => Number(t.id) !== Number(order.assignee_id));
  const option = (t: Row) => <option key={t.id} value={t.id}>{t.name} · {t.assigned_count} atandı / {t.in_progress_count} işlemde / {t.on_hold_count} beklemede</option>;
  return <div className="workflow-actions">
    {order.hold_reason && <div className="notice">Bekleme nedeni: {order.hold_reason}</div>}
    {order.reassignment_requested && active && <div className="notice">Devir talebi yönetici değerlendirmesini bekliyor: {order.reassignment_reason}</div>}
    {manager && order.status === 'open' && <form onSubmit={e => submit(e, { status: 'assigned' })}><h3>Teknisyene ata</h3><label>Sorumlu teknisyen<select name="assignee_id" required disabled={busy}><option value="">İş yükünü değerlendirerek seçin</option>{technicians.map(option)}</select></label><label>Atama notu<textarea name="note" maxLength={10000} rows={2} /></label><button disabled={busy || !technicians.length}>İşi ata</button></form>}
    {owner && (order.status === 'assigned' || order.status === 'on_hold') && <form onSubmit={e => submit(e, { status: 'in_progress' })}><h3>{order.status === 'on_hold' ? 'Müdahaleye devam et' : 'Müsait olduğunuzda işe başlayın'}</h3><button disabled={busy}>{order.status === 'on_hold' ? 'İşe devam et' : 'İşe başla'}</button></form>}
    {owner && order.status === 'in_progress' && <><form onSubmit={e => submit(e, { status: 'resolved' })}><h3>Çözümü yöneticinin onayına gönder</h3><label>Yapılan işlem ve çözüm<textarea name="note" required maxLength={10000} rows={3} /></label><button disabled={busy}>Çözüldü olarak gönder</button></form><form onSubmit={e => submit(e, { status: 'on_hold' })}><h3>Müdahaleyi beklemeye al</h3><label>Bekleme gerekçesi<textarea name="note" placeholder="Örn. yedek parça veya saha erişimi bekleniyor" required maxLength={10000} rows={2} /></label><button className="secondary" disabled={busy}>Beklemeye al</button></form></>}
    {owner && active && !order.reassignment_requested && <details><summary>Bu işi üstlenemiyor musunuz?</summary><form onSubmit={e => submit(e, { action: 'request_reassignment' })}><label>Yeniden atama gerekçesi<textarea name="note" required maxLength={10000} rows={2} /></label><button className="secondary" disabled={busy}>Yöneticiye devir talebi ilet</button></form><small>Yönetici yeniden atayana kadar işin sorumluluğu sizde kalır.</small></details>}
    {manager && active && <details><summary>Başka teknisyene yeniden ata</summary><form onSubmit={e => submit(e, { action: 'reassign' })}><label>Yeni sorumlu<select name="assignee_id" required disabled={busy}><option value="">Farklı bir teknisyen seçin</option>{options.map(option)}</select></label><label>Yeniden atama gerekçesi<textarea name="note" required maxLength={10000} rows={2} /></label><button disabled={busy || !options.length}>Yeniden ata</button></form></details>}
    {manager && order.status === 'resolved' && <><form onSubmit={e => submit(e, { status: 'closed' })}><h3>Çözümü kontrol et</h3><label>Kontrol notu<textarea name="note" maxLength={10000} rows={2} /></label><button disabled={busy}>Onayla ve kapat</button></form><details><summary>Çözümü teknisyene geri gönder</summary><form onSubmit={e => submit(e, { action: 'return' })}><label>Eksik veya düzeltilmesi gereken işlem<textarea name="note" required maxLength={10000} rows={2} /></label><button className="secondary" disabled={busy}>Gerekçeyle geri gönder</button></form></details></>}
    {manager && active && <p className="muted">Müdahale durumunu sorumlu teknisyen günceller. Gerektiğinde işi başka teknisyene atayabilirsiniz.</p>}
    {owner && order.status === 'resolved' && <p className="notice">Çözümünüz yönetici onayını bekliyor.</p>}
  </div>;
}
