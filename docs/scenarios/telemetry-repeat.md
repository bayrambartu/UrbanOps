# İlk arızadan tekrar olayına çözüm aktarımı

Bu kayıt, gerçek cihaz müdahalesi içermeyen bir demo senaryosudur. Amaç, ilk olayda bilinmeyen bir hata kodunun onaylı çözümünün tekrar olayında kaynak olarak bulunmasını sınamaktır.

- Cihaz: TRF-BAG-001, Bağcılar.
- Yeni hata kodu: TELEMETRY_INTERVAL_MISMATCH. Deney öncesinde kayıtlı arızalarda bu kodun bulunmadığı kontrol edildi.
- İlk arıza / iş: #10 / İE-10.
- Tekrar arızası / iş: #11 / İE-11.

## Adımlar

1. İlk manuel bildirim açıldı. Bakım asistanı `no_sources` döndürdü; geçmiş çözüm yoktu.
2. Yönetici İE-10'u Ayhan Kurnaz'a atadı. Ayhan kendi hesabıyla işi başlattı ve demo çözümünü kaydetti.
3. Çözümdeki kök neden, telemetri yayın aralığının 60 saniye olmasına rağmen izleme panelinin 10 saniyede veri beklemesi olarak tanımlandı. DEMO-TLM-03 profilinde yayın aralığı 10 saniye, uyarı eşiği 30 saniye olarak eşleştirilmiş varsayıldı. Altı ardışık mesajla doğrulama bir eğitim varsayımıdır; fiziksel ölçüm yapılmadı.
4. Çözülmüş fakat yönetici tarafından kapatılmamışken asistan yeniden `no_sources` döndürdü. Böylece onaysız çözümün ortak bilgiye dönüşmediği kontrol edildi.
5. Yönetici demo çözümünü onaylayıp İE-10'u kapattı.
6. Aynı cihaz ve hata koduyla yeni bir bildirim açıldı. Yeni arıza/iş kimliği 11 oldu; kapanmış kayıt tekrar açılmadı.
7. İE-11, `bartu@gmail.com` hesabındaki teknisyen Bartu Kurnaz'a atandı. Parolası değiştirilmedi; bu iş bizim tarafımızdan başlatılmadı veya çözülmedi.
8. `/api/faults/11/similar`, ilk sırada Ayhan'ın yönetici onaylı #10 çözümünü getirdi. `/suggestions` için kaynak kimliği `fault-10` oldu.

## Son kullanıcı kontrolü

Bartu hesabıyla **İşlerim → İE-11 → İşi aç → Bakım asistanı → Kaynaklı öneri getir**.

Beklenen yanıt, geçmiş kök nedeni ve DEMO-TLM-03 / 10 saniye / 30 saniye bilgilerini kaynakla birlikte içermelidir. Kaynak detayındaki `fault-10` ve **Onaylı geçmiş çözümler** bölümündeki İE-10 aynı geçmiş kaydı göstermelidir. Aynı kök neden yeni olay için yeniden doğrulanmalıdır.

Bu denemede geçmiş çözümler aynı hata koduyla ilişkisel olarak getirilir; aynı cihaz önceliklidir. Yeni kullanıcıya aynı kayıtların görünmesi, bakım asistanının kaynak erişiminden yararlanmasıdır. Model cevabının tüm olası arızalarda teknik olarak doğru olduğu anlamına gelmez.

## AI bağlantısı

Bilgisayarda zaten bulunan Ollama modelleri kullanıldı. Docker içindeki API, `host.docker.internal:11434` adresinden host servisine bağlanır. Chat modeli `qwen3:4b`, embedding modeli `bge-m3:latest` olarak seçildi. Modelin kaynak ayrıntılarını koruması için yanıt talimatları ve yalnızca getirilen kaynak kimliklerini kabul eden JSON şeması güncellendi. Şema, referans biçimini sınırlar; teknik doğruluk ayrıca kaynakla karşılaştırılır.

## Canlı testte bulunan ek bağımlılık

İlk gerçek AI isteği `generated` döndürdü ve `fault-10` kaynağını kullandı; yanıt teknik ayarları yeterince ayrıntılı aktarmadığı için talimatlar ve çıktı şeması iyileştirildi. Yerel Qwen3 testinde doğru profil, 10/30 saniye, altı mesajla doğrulama ve demo etiketi kaynakla eşleşti.

API üzerinden sonraki denemede zaman aşımı gözlendi. Belgelerde henüz vektör parçası olmamasına rağmen embedding modelinin çağrıldığı tespit edildi. Artık ilgili embedding modeline ait vektör yoksa bu çağrı yapılmaz; onaylı geçmiş çözümler ve metninde hata kodu geçen belgeler doğrudan üretim modeline bağlam olur. Vektörler varsa embedding araması denenir; bu arama başarısız olduğunda mevcut ilişkisel kaynaklar kullanılmaya devam eder. Gerçek generation hatası ise `sources_only` moduna dönülür ve sunucu loguna kaydedilir.

## Doğrulanan son sonuç

Düzeltme çalışan API'ye uygulandıktan sonra `/api/faults/11/suggestions`, gerçek yerel modelle **generated** döndürdü. Tek kaynak ve citation **fault-10** oldu. Yanıtta **DEMO-TLM-03**, **10 saniye**, **30 saniye**, **6 ardışık mesajla doğrulama** ve **simülasyon etiketi** kaynakla eşleşti. API isteği yaklaşık **52,67 saniye** sürdü.

Son çıktıdan çözüm bölümü:

> Çözüm: DEMO-TLM-03 profilinde yayın aralığı 10 saniye ve izleme uyarı eşiği 30 saniye.

Bu kontrol yönetici oturumundan aynı bakım asistanı API'si çağrılarak yapıldı. Bartu hesabındaki son arayüz tıklamasını kullanıcı yapmayı seçti; Bartu'nun parolası alınmadı veya değiştirilmedi. İE-11 teknisyenin denemesi için atanmış bırakıldı.
