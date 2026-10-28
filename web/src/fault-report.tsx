import { useRef, useState } from 'react';
type Row = Record<string, any>;
const kinds = [
  ['CONNECTION_TIMEOUT', 'Bağlantı zaman aşımı'], ['LINK_CRC_ERRORS', 'Ethernet / CRC hatası'],
  ['DHCP_ADDRESS_FAILURE', 'IP adresi alınamıyor'], ['POWER_FAILURE', 'Güç / besleme sorunu'],
  ['SENSOR_FAILURE', 'Sensör hatası'], ['CUSTOM', 'Diğer / özel hata kodu'],
];
const localTime = () => { const now = new Date(); return new Date(now.getTime() - now.getTimezoneOffset() * 60000).toISOString().slice(0, 16); };
const statusName: Record<string, string> = { open: 'Atama bekliyor', assigned: 'Atandı', in_progress: 'İşlemde', on_hold: 'Beklemede', resolved: 'Çözüm onayında', closed: 'Kapatıldı' };
export function FaultReportForm({ devices, onSubmit }: { devices: Row[]; onSubmit: (payload: Row) => Promise<Row> }) {
  const [deviceID, setDeviceID] = useState(''); const [kind, setKind] = useState('CONNECTION_TIMEOUT');
  const [customCode, setCustomCode] = useState(''); const [severity, setSeverity] = useState('medium');
  const [description, setDescription] = useState(''); const [observed, setObserved] = useState(localTime);
  const [locationDetail, setLocationDetail] = useState(''); const [note, setNote] = useState('');
  const [error, setError] = useState(''); const [pending, setPending] = useState(false); const [receipt, setReceipt] = useState<Row | null>(null);
  const attempt = useRef<{ body: string; id: string } | null>(null);
  const device = devices.find(d => d.id === deviceID);
  async function submit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault(); if (pending || receipt) return; setError('');
    if (new TextEncoder().encode(description.trim()).length > 2000) { setError('Açıklama çok uzun; lütfen kısaltın.'); return; }
    const payload: Row = { device_id: deviceID, fault_code: kind === 'CUSTOM' ? customCode.trim().toUpperCase() : kind, severity, description: description.trim(), location_detail: locationDetail.trim(), field_note: note.trim() };
    if (observed) payload.observed_at = new Date(observed).toISOString();
    const body = JSON.stringify(payload);
    if (!attempt.current || attempt.current.body !== body) attempt.current = { body, id: crypto.randomUUID() };
    setPending(true);
    try { setReceipt(await onSubmit({ ...payload, request_id: attempt.current.id })); }
    catch (e) { setError((e as Error).message); }
    finally { setPending(false); }
  }
  function reset() { setReceipt(null); setDescription(''); setLocationDetail(''); setNote(''); setObserved(localTime()); attempt.current = null; setError(''); }
  return <section className="panel report-form"><h2>Manuel arıza bildirimi</h2><p className="muted">Sahada gözlemlediğiniz sorunu kaydedin. Bildiren kişi ve kayıt zamanı hesabınız üzerinden otomatik tutulur. Atamayı yönetici yapar.</p>
    {error && <div className="alert" role="alert">{error}</div>}
    {receipt && <div className="notice" role="status"><strong>{receipt.duplicate ? 'Bu bildirim daha önce kaydedilmiş.' : receipt.created_fault ? 'Yeni arıza ve iş emri oluşturuldu.' : 'Bildiriminiz mevcut açık arızaya eklendi.'}</strong><p>Arıza #{receipt.fault_id} · İş emri İE-{receipt.work_order_id}</p><small>{receipt.created_fault ? 'İş emri yönetici atamasını bekliyor.' : 'Mevcut iş emri ve sorumlu personel korundu.'}</small><button type="button" className="secondary" onClick={reset}>Yeni bildirim oluştur</button></div>}
    {!devices.length && <div className="notice">Bildirim açmak için kayıtlı cihaz gerekli. Yöneticinizden cihazı envantere eklemesini isteyin.</div>}
    <form onSubmit={submit}><fieldset disabled={pending || !!receipt || !devices.length}><div className="report-grid">
      <label>Cihaz *<select required value={deviceID} onChange={e => setDeviceID(e.target.value)}><option value="">Cihaz seçin</option>{devices.map(d => <option key={d.id} value={d.id}>{d.id} · {d.name} · {d.location}</option>)}</select></label>
      <label>Kayıtlı konum<input value={device?.location || ''} readOnly placeholder="Cihaz seçildiğinde otomatik gelir" /></label>
      <label>Hata türü *<select value={kind} onChange={e => setKind(e.target.value)}>{kinds.map(([code, label]) => <option key={code} value={code}>{label}</option>)}</select></label>
      <label>Önem derecesi *<select value={severity} onChange={e => setSeverity(e.target.value)}><option value="low">Düşük — küçük etki</option><option value="medium">Orta — kısmi aksama</option><option value="high">Yüksek — hizmet kesintisi</option><option value="critical">Kritik — yaygın / acil etki</option></select></label>
      {kind === 'CUSTOM' && <label>Özel hata kodu *<input required minLength={3} maxLength={64} pattern="[A-Z][A-Z0-9_]{2,63}" value={customCode} onChange={e => setCustomCode(e.target.value.toUpperCase())} placeholder="Örn. DISPLAY_FAILURE" /><small>Tekrarlayan aynı sorunlarda aynı kodu kullanın.</small></label>}
      <label>Gözlem zamanı<input type="datetime-local" value={observed} onChange={e => setObserved(e.target.value)} /><small>Boş bırakılırsa kayıt anı kullanılır.</small></label>
      <label>Konum detayı<input maxLength={300} value={locationDetail} onChange={e => setLocationDetail(e.target.value)} placeholder="Kavşak, yön, direk veya pano numarası" /></label>
    </div><label>Arıza açıklaması *<textarea required minLength={10} maxLength={1000} rows={5} value={description} onChange={e => setDescription(e.target.value)} placeholder="Ne gözlemlediniz? Sorun ne zaman ve hangi koşullarda oluşuyor? Hizmete etkisi nedir?" /><small>En az 10 karakter. Belirti ve varsa hata mesajını açıkça yazın.</small></label>
    <label>Saha notu / ilk kontroller<textarea maxLength={1000} rows={3} value={note} onChange={e => setNote(e.target.value)} placeholder="Yapılan ilk kontroller, sorunun sıklığı veya müdahaleyi etkileyen koşullar (isteğe bağlı)" /></label>
    <button type="submit">{pending ? 'Kaydediliyor…' : 'Arıza bildirimi gönder'}</button><p className="muted">Aynı cihaz ve hata kodunda açık arıza varsa gözleminiz o kayda eklenir; yeni iş emri oluşturulmaz.</p></fieldset></form>
  </section>;
}
export function ManualReports({ reports, manager, onInspect }: { reports: Row[]; manager: boolean; onInspect: (report: Row) => void }) {
  return <section className="panel"><h2>{manager ? 'Manuel bildirimler' : 'Bildirimlerim'}</h2><p className="muted">{manager ? 'Personelden gelen saha gözlemleri ve bağlı iş emirleri.' : 'Kendi açtığınız bildirimleri takip edin. Bildirim açmak işi otomatik olarak size atamaz.'}</p>
    {!reports.length && <div className="empty"><p>Henüz manuel bildirim yok.</p></div>}
    {reports.map(r => <article className="manual-report" key={r.event_id}><div className="panel-heading"><div><strong>{r.device_id} · {r.fault_code}</strong><small>{r.location} · {r.location_detail || 'Ek konum detayı yok'}</small></div><span className={'badge ' + r.work_order_status}>{statusName[r.work_order_status] || r.work_order_status}</span></div><p className="technical-text">{r.description}</p>{r.field_note && <p className="muted">Saha notu: {r.field_note}</p>}<small>Bildiren: {r.reporter} · Gözlem: {new Date(r.observed_at).toLocaleString('tr-TR')} · Kayıt: {new Date(r.received_at).toLocaleString('tr-TR')}</small><small>Arıza #{r.fault_id} · İş emri İE-{r.work_order_id}</small>{manager && <button className="text" onClick={() => onInspect(r)}>Arızayı incele ↗</button>}</article>)}
  </section>;
}
export function FaultEventHistory({ events }: { events: Row[] }) {
  const source: Record<string, string> = { manual: 'Manuel bildirim', device: 'Cihaz / otomatik', simulator: 'Simülatör' };
  return <section className="fault-event-history"><h3>Arıza gözlemleri ve bildirenler</h3><small>Son 100 gözlem gösterilir. Otomatik olaylarda personel kimliği bulunmaz.</small>{events.map(e => <details key={e.event_id}><summary>{source[e.source] || e.source} · {e.reporter || 'Sistem'} · {new Date(e.received_at).toLocaleString('tr-TR')}</summary><small>Gözlem zamanı: {new Date(e.observed_at).toLocaleString('tr-TR')}</small><p className="technical-text">{e.description}</p>{e.location_detail && <p>Konum detayı: {e.location_detail}</p>}{e.field_note && <p>Saha notu: {e.field_note}</p>}</details>)}</section>;
}
