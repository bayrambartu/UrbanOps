import { useState } from 'react';
type Row = Record<string, any>;
export function PastSolutions({ records }: { records: Row[] }) {
  return <section className="past-solutions"><h3>Onaylı geçmiş çözümler</h3><p className="muted">Aynı hata kodunda yönetici tarafından kapatılmış işler. Model çalışmadan da okunabilir; geçmişteki kök neden yeni arızada tekrar doğrulanmalıdır.</p>
    {records.map(r => <details key={r.fault_id}><summary>İE-{r.work_order_id} · {r.device_id} · {r.same_device ? 'Aynı cihaz' : 'Aynı hata kodu'}</summary><small>{new Date(r.resolved_at).toLocaleString('tr-TR')} · Teknisyen: {r.technician || 'Belirtilmemiş'}</small><p className="technical-text">{r.resolution}</p><div className="notice"><strong>Yönetici onayı: {r.approved_by || 'Onaylı kapalı kayıt'}</strong><p>{r.approval_note || 'Ek kontrol notu bulunmuyor.'}</p></div></details>)}
    {!records.length && <p className="muted">Bu hata kodunda henüz onaylanmış geçmiş çözüm bulunamadı.</p>}
    <details><summary>Yeni personel için inceleme sırası</summary><ol><li>Arızanın güncel belirtisini ve ölçümlerini kaydedin.</li><li>Geçmiş çözümdeki gözlem, kök neden ve doğrulamayı ayırın.</li><li>Teknik kaynakla karşılaştırarak kendi kontrol planınızı oluşturun.</li><li>Müsait olduğunuzda işe başlayın; sonuç ve doğrulama kanıtını çözüm notuna yazın.</li></ol><small>RCA: kök neden analizi · CRC: iletim bütünlüğü kontrolü · RTO: TCP yeniden iletim zaman aşımı · DORA: DHCP mesaj akışı.</small></details>
  </section>;
}
function SourceLink({ content }: { content: string }) {
  const url = content.match(/^URL:\s*(https:\/\/\S+)\s*$/m)?.[1];
  return url ? <a href={url} target="_blank" rel="noreferrer">Asıl teknik kaynağı aç ↗</a> : null;
}
export function KnowledgeDocuments({ documents, manager, busy, onRead, onIndex }: { documents: Row[]; manager: boolean; busy: boolean; onRead: (id: number) => Promise<Row>; onIndex: (id: number) => Promise<void> }) {
  const [contents, setContents] = useState<Record<number, Row>>({});
  const [loading, setLoading] = useState<number | null>(null);
  const [error, setError] = useState('');
  async function read(id: number) {
    if (contents[id]) return;
    setError(''); setLoading(id);
    try { const document = await onRead(id); setContents(previous => ({ ...previous, [id]: document })); }
    catch (e) { setError((e as Error).message); }
    finally { setLoading(null); }
  }
  return <section className="panel"><h2>Teknik kaynaklar</h2><p className="muted">Belgeler model kapalıyken de saklanır ve okunabilir. Vektör araması için indeks oluşturulmalıdır.</p>{error && <div role="alert" className="alert">{error}</div>}
    {documents.map(d => <article className="knowledge-document" key={d.id}><details onToggle={e => { if (e.currentTarget.open) void read(d.id); }}><summary>{d.title}</summary><small>{Number(d.chunks) ? `${d.chunks} vektör parçası · İndeksli` : 'Metin kayıtlı · Vektör indeksi bekliyor'} · {new Date(d.created_at).toLocaleDateString('tr-TR')}</small>{loading === d.id && <p>Belge okunuyor…</p>}{contents[d.id] && <><SourceLink content={contents[d.id].content} /><p className="technical-text">{contents[d.id].content}</p></>}</details>{manager && !Number(d.chunks) && <button className="secondary" disabled={busy} onClick={() => void onIndex(d.id)}>Yeniden indeksle</button>}</article>)}
    {!documents.length && <p className="muted">Henüz belge kaydı yok.</p>}
  </section>;
}
