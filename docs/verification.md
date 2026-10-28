# Doğrulama durumu

Tarih: 28 Ekim 2026. Bu dosya uygulamanın hangi parçalarının bu geliştirme ortamında doğrulandığını kaydeder.

## Başarılı kontroller

- `go test -race -count=1 ./...`: yedi birim test fonksiyonu ve alt senaryoları geçti. Arıza doğrulama, tüm iş emri durum çiftleri, JWT imzası/süresi/değiştirme girişimi, katı JSON okuma, giriş sınırı, Unicode parçalama ve embedding yanıt doğrulaması kapsandı.
- `go vet ./...`: hata yok.
- `go build ./...`: API, worker ve simülatör derlendi.
- `npm ci` (yerel paket önbelleğiyle) ve `npm run build`: TypeScript kontrolü ve Vite üretim derlemesi başarılı.
- Tüm profillerle `docker compose config --quiet`: Compose yapılandırması geçerli.
- Python kurulum/demo dosyaları derleme kontrolünden geçti; kurulum komutu rastgele anahtarlarla `.env` oluşturdu.

## Burada çalıştırılamayan kontroller

- Docker daemon soketine erişim ortam tarafından engellendi. Image build ve Compose servis başlangıcı yapılmadı.
- Yerel PostgreSQL başlatma denemesi paylaşımlı bellek izni nedeniyle başarısız oldu. Veritabanı şeması ve SQL sorguları gerçek PostgreSQL üzerinde henüz doğrulanmadı.
- `TestDurableIngestionAndWorkOrderLifecycle`, `TEST_DATABASE_URL` ve `TEST_RABBITMQ_URL` verilmediğinden **SKIP** oldu. Test dosyasının derlenmesi, entegrasyonun geçtiği anlamına gelmez.
- Ollama modelleri indirilmedi; gerçek model ile belge indeksleme/öneri kalitesi değerlendirilmedi.
- Bu ortam yerel port dinlemeyi engellediği için tarayıcıyla görsel/etkileşimli panel kontrolü yapılmadı.

## Sonraki doğrulama

Docker erişimi bulunan bir ortamda:

```sh
docker compose --profile test run --build --rm integration
docker compose up --build -d
python3 scripts/demo.py
```

Panelde arızayı personele atayın, başlatın, çözüm notuyla çözün ve kapatın. Ardından yeni bir olay gönderip yeni arıza oluşmasını doğrulayın. AI profiliyle model kurulumundan sonra gerçek teknik belgeler ve değerlendirme sorularıyla kaynak/yanıt doğruluğunu ölçün. Retry, DLQ ve bağlantı kopması için process seviyesinde hata enjeksiyon testleri ayrıca gereklidir.

## İş emri hiyerarşisi güncellemesi (v0.2)

- `go test -race -count=1 -v ./...`: sekiz birim test fonksiyonu geçti. Yeni iş akışı politikası için 33 alt senaryo; yönetici/teknisyen yetkileri, bekleme gerekçesi, devir talebi, yeniden atama ve çözümün geri gönderilmesini kapsar.
- `go vet ./...`, TypeScript/Vite üretim derlemesi ve Compose yapılandırma kontrolü başarılı.
- Gerçek servis entegrasyon testi yeni hiyerarşi için genişletildi; bağlantı değişkenleri olmadığından yine **SKIP** oldu. Migration 002 ve HTTP/SQL akışı gerçek PostgreSQL üzerinde bu ortamda çalıştırılmadı.
- `docker compose up --build -d api worker` denendi; Docker soketi erişimi ortam tarafından engellendi. Çalışan konteynerler bu oturumda güncellenemedi. Yerelde aynı komut çalıştırılıp tarayıcı yenilenmelidir.

## Yeni personel eğitim seti ve kaynak erişimi

Çalışan API üzerinden doğrulananlar:

- Her rolden üç yeni kurgu personel oluşturuldu (toplam 9). Mevcut hesaplar değiştirilmedi. Yeni teknisyen hesabıyla giriş başarılı.
- Eğitim geçmişi: İE-2 CONNECTION_TIMEOUT, İE-3 LINK_CRC_ERRORS, İE-4 DHCP_ADDRESS_FAILURE. Üçü de kıdemli teknisyen tarafından çözülüp ilgili yönetici hesabıyla kapatıldı.
- Yeni personele atanan işler: İE-5, İE-6, İE-7. Ali Yıldız kendi listesinden yalnızca bu üç işi görüyor. Başka teknisyenin tam işlem geçmişine doğrudan erişim 404 döndürdü.
- Kullanıcı konteyner güncellemesini uyguladıktan sonra üç araştırılmış kaynak belgesi başlangıç aktarımıyla veritabanına kaydedildi. Belgeler `pending` (vektör indeksi yok) durumunda ve yeni personel tarafından okunabiliyor.
- Üç yeni arıza için `/similar` ilk sırada aynı cihazın onaylı çözümünü, yönetici adını ve kontrol notunu getiriyor. Sistemde daha önceden bulunan, aynı hata kodundaki onaylı kayıtlar da korunuyor ve sonraki sıralarda görünebiliyor.
- Üç `/suggestions` isteği de HTTP 200 ve `sources_only` döndürdü; onaylı geçmiş kayıt ile ilgili teknik belge birlikte geldi. Ollama yanıtı veya gerçek model kalitesi test edilmiş sayılmaz; yanıtlar kaynak görünümüdür.
- Aynı teknik belge ikinci kez POST edildiğinde HTTP 202 ile aynı belge kimliği döndü; toplam belge sayısı 3 kaldı. Model kapalıyken kalıcı kayıt ve idempotency canlı API'de doğrulandı.

Kaynak kod kontrolleri: `go test -race -count=1 ./...` (10 birim test fonksiyonu ve alt senaryolar), `go vet`, `go build`, TypeScript/Vite üretim derlemesi, Python derleme kontrolü ve Compose yapılandırma kontrolü başarılı. Yeni kaynak kimliği doğrulama testi, uydurma kaynak/boş cevap/bozuk JSON çıktılarının model yanıtı olarak gösterilmediğini doğrular. İzole Docker entegrasyon testi ortam değişkenleri olmadığından halen SKIP; bu durum canlı API üzerinden yapılan kontrollerden ayrıdır.

## Manuel bildirim güncellemesi — kod doğrulaması

- `go test -race -count=1 ./...`: 13 birim test fonksiyonu ve alt senaryoları geçti. Manuel alan sınırları, Unicode açıklama uzunluğu, gelecek zaman, anahtar/kod doğrulama, normalize edilmiş tekrar deneme özeti ve otomatik olayın manuel kaynak iddia edememesi kapsanır.
- `go vet`, `go build`, TypeScript/Vite üretim derlemesi ve Compose yapılandırma kontrolü başarılı.
- `TestManualReportsAndProvenance` eklendi: tüm rollerden bildirim, kimlik sahteciliğinin reddi, eşzamanlı tekrar denemeler, değişen içerikte 409, aynı arızaya birleştirme, atamanın korunması, otomatik/manüel ortak akış, kişisel bildirim görünürlüğü ve audit kayıtlarını gerçek PostgreSQL ile kontrol eder. TEST_DATABASE_URL olmadığından bu ortamda SKIP oldu; derlenmesi gerçek entegrasyonun geçtiği anlamına gelmez.
- Çalışan API `/api/fault-reports` için halen 404 döndürüyor. Konteyner güncellemesi bekleniyor; migration 003 ve yeni uç noktalar canlı veritabanında henüz doğrulanmadı. Güncelleme komutu: `docker compose up --build -d api worker`.

## İlk hata → onaylı çözüm → Bartu'ya tekrar atama

Canlı API'de TELEMETRY_INTERVAL_MISMATCH hata kodunun başlangıçta bulunmadığı kontrol edildi. İlk bildirim #10 / İE-10 olarak oluşturuldu; ilk asistan sonucu `no_sources` oldu. Ayhan'ın kendi oturumuyla iş başlatıldı ve demo çözümü kaydedildi. Yönetici onayı öncesindeki kontrol yine `no_sources` döndürdü. Yönetici İE-10'u kapattı. Aynı cihaz/kodla yeni bildirim #11 / İE-11 oluşturuldu ve mevcut Bartu Kurnaz teknisyenine (bartu@gmail.com) atandı.

İlk AI denemesi `generated` ve `fault-10` döndürdü ancak çözüm ayrıntılarını fazla özetledi. Yanıt talimatları ve bilinen kaynak kimliklerini sınırlayan JSON şeması eklendi. Sonraki gerçek model denemesinde zaman aşımı görüldü; vektörlü belge yokken embedding çağrısı yapılması kaldırıldı ve model çağrı hatalarına yapılandırılmış log eklendi.

Son canlı kontrol: HTTP 200, **mode=generated**, **citations=[fault-10]**. Yanıtın DEMO-TLM-03 / 10 saniye / 30 saniye / 6 ardışık mesaj / simülasyon bilgileri, Ayhan'ın kayıtlı çözümüyle karşılaştırıldı ve eşleşti. Kaynağın içeriği, orijinal çözüm notunu aynen içeriyor. Süre yaklaşık 52,67 saniye. Bu dar senaryo için gerçek LLM üretimi doğrulandı; tüm arızalarda teknik doğruluk garantisi anlamına gelmez.

Kontrol yönetici hesabından yapıldı. Bartu hesabındaki son UI adımı kullanıcıya bırakıldı; parolası değiştirilmedi. Ayrıntılar: docs/scenarios/telemetry-repeat.md.

Kod kontrolü: `go test -race -count=1 ./...` ve `go vet ./...` başarılı. 14 birim test fonksiyonu ve alt senaryoları geçti; iki gerçek servis entegrasyon testi bağlantı değişkenleri olmadığından SKIP. Entegrasyon testine, vektör yokken yalnızca generation çağrısı yapılması için regresyon kontrolü de eklendi; bu regresyonun canlı karşılığı yukarıdaki gerçek API denemesinde gözlendi.
