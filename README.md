# UrbanOps AI

Go, PostgreSQL, RabbitMQ ve yerel RAG ile arıza takip ve bakım yönetimi prototipi. İSBAK veya İBB ile resmî iş birliği/entegrasyon içermez; gerçek şehir altyapısına bağlı değildir.

## Hızlı başlangıç

Gereksinimler: Docker Engine/Desktop + Compose, Python 3. Yerel kaynak geliştirmesi için Go 1.22+ ve Node.js 22+.

```sh
python3 scripts/setup.py
docker compose up --build -d
python3 scripts/demo.py
```

PostgreSQL yerel erişim adresi: `localhost:5435` (konteyner içinde `5432`).

Panel: **http://localhost:8080**. İlk yönetici e-postası ve parolası `.env` içindeki `ADMIN_EMAIL` / `ADMIN_PASSWORD` alanlarında. Setup her kurulumda rastgele anahtarlar üretir; mevcut `.env` üzerine yazmaz. `.env.example` yalnızca değişken referansıdır. Yönetici parolasının sonradan `.env` içinde değiştirilmesi mevcut hesabı değiştirmez; bootstrap yalnızca ilk oluşturmayı yapar.

Demo, `TRF-104` cihazını ve atama yapılabilen demo teknisyeni oluşturur; aynı olayı iki kez gönderir. Paneli yenilediğinizde tek olay ve tek iş emri görmelisiniz. Yeni olay kimlikleri açık arızanın tekrar sayısını artırır. Demo hesabının rastgele parolası saklanmaz; teknisyen olarak giriş yapmak için yönetici panelinden kendi teknisyen hesabınızı oluşturun.

Sürekli simülasyon (önce demo ile cihazı oluşturun):

```sh
docker compose --profile simulation up -d simulator
```

Simülatörü durdurma: `docker compose stop simulator`. Tüm servisleri durdurma: `docker compose down`. Veriler adlandırılmış volume'larda korunur.

## Neler var?

- Türkçe React/TypeScript panel: genel bakış, filtrelenebilir arıza listesi, cihaz ekleme, iş emirleri, ekip, bilgi bankası ve raporlar.
- Go HTTP API, JWT oturumu (8 saat), bcrypt, yönetici/operatör/teknisyen yetkileri ve process başına giriş sınırı.
- Kalıcı RabbitMQ mesajları, publisher confirm sonrası `202`, PostgreSQL commit sonrası consumer ACK.
- Sınırlı retry (5 saniye gecikmeyle 3 yeniden deneme), dead-letter kuyruğu, worker yeniden bağlanma döngüsü.
- Transaction ve `event_id` birincil anahtarıyla idempotency; eşzamanlı tüketiciler için advisory lock.
- Aynı cihaz/hata kodunda tek açık arıza; önem derecesi yalnızca yükselir, olay zamanları sıradan bağımsız birleştirilir.
- Ayrı arıza ve iş emri durumları, zorunlu çözüm notu, kullanıcı ve zaman bilgili durum geçmişi.
- pgvector belge araması, aynı hata kodundaki geçmiş çözüm kayıtları, Ollama kaynaklı bakım önerisi.
- Temel raporlar, SQL migration, OpenAPI ve otomatik testler.

## Bakım asistanını açma

```sh
docker compose --profile ai up -d ollama
docker compose exec ollama ollama pull nomic-embed-text
docker compose exec ollama ollama pull qwen2.5:7b
```

Panelin **Bilgi bankası** ekranından teknik metin ekleyin. Arıza detayından **Kaynaklı öneri getir** seçeneğini kullanın. AI kapalıyken arıza, iş emri, belge okuma ve onaylı geçmiş çözüm akışı çalışmaya devam eder. Asistan `sources_only` modunda geçmiş çözümleri ve hata koduyla tam eşleşen belgeleri gösterir; bu mod LLM yanıtı değildir. Modellerin indirilmesi internet ve yeterli disk/RAM gerektirir. macOS'ta Docker içindeki model CPU üzerinde yavaş olabilir; yerel Ollama kullanmak için API'nin `OLLAMA_URL` değerini `http://host.docker.internal:11434` yapabilirsiniz.

Belge içeriği 900 Unicode karakterlik, 200 karakter örtüşen parçalara ayrılır. Embedding/model adı birlikte saklanır. Belge araması cosine benzerliğiyle yapılır; ilk eşik 0.45'tir ve pilot verilerle ayarlanmalıdır. Geçmiş çözümler yalnızca yönetici tarafından kapatılmış işlerden **aynı hata koduyla ilişkisel sorgulanır**; geçmiş kayıtlar için anlamsal embedding hattı henüz yoktur. Embedding modeli değişirse belgeleri yeni modelle yeniden ekleyin.

Belgeler model kapalıyken de kalıcı kaydedilir ve okunabilir; indeksleme başarısızsa API `202 pending` döndürür. Otomatik indeksleme kuyruğu yoktur; model hazır olduğunda panelden **Yeniden indeksle** seçilir.

Kaynak bulunamazsa öneri oluşturulmaz. Yanıttaki kaynak kimlikleri backend tarafından kontrol edilir. Bu kontrol önerinin teknik doğruluğunu kanıtlamaz; bakım müdahalesi yetkili personelin değerlendirmesine bağlıdır. Modele, kaynaklar ve arıza metinleri içindeki talimatlara uymaması söylenir; bu tek başına prompt injection korumasını kanıtlamaz. PDF/DOCX ayrıştırma henüz yok; metin yapıştırma ve `.txt` / `.md` dosyası okuma desteklenir.

## İş emri akışı ve hiyerarşi

`Açık → Atandı → İşlemde → Çözüldü (yönetici onayı) → Kapatıldı`

- **Yönetici / operatör:** Teknisyenlerin atanan, işlemde ve bekleyen iş sayılarını görerek atama yapar. Çözümü kontrol edip kapatır; gerekçeyle sorumlu teknisyene geri gönderebilir. Teknisyen adına işe başlayamaz veya çözüm giremez.
- **Teknisyen:** Yalnızca kendisine atanmış işlerin listesini, bağlı arızalarını, işlem geçmişini ve bakım önerilerini görür. **İşlerim** ekranından müsait olduğunda işe başlar; işleri önem ve oluşturulma zamanına göre sıralanır.
- **Beklemede:** İşlemdeki iş gerekçeyle bekletilebilir. Engel kalkınca teknisyen tekrar işlemde durumuna geçirir. Bekleyen iş çözülmeden önce yeniden başlatılır.
- **Devir talebi:** Teknisyen gerekçe girer; sorumluluk ve mevcut durum değişmez. Yönetici değerlendirdikten sonra farklı teknisyene yeniden atar. Yeni personel işi `Atandı` durumunda alır. Yeni atama, eski bekleme ve devir talebini temizler. Teknisyen işi çözerse açık devir talebi de kapanır.
- **Çözümü geri gönderme:** Yönetici gerekçesiyle işi aynı teknisyene `Atandı` durumunda döndürür; arıza yeniden açılır. Aynı cihaz/hata için daha yeni bir açık arıza varsa çakışma oluşmaması için işlem reddedilir; yeni iş üzerinden devam edilir.
- **İşlem geçmişi:** Her değişiklikte aktör, zaman, gerekçe ve eski/yeni sorumlu kaydedilir. Eski çözüm notları geri gönderme sonrasında da geçmişte korunur.

İş yükü sayıları vardiya veya fiziksel müsaitlik beyanı değildir; atama kararını yönetici verir. Teknisyenlerin diğer ekiplerin işlerini görmesi veya yönetim raporlarına erişmesi backend tarafından da engellenir. Teknik bilgi bankası ortak erişime açıktır.

### Mevcut kurulumu güncelleme

```sh
docker compose up --build -d api worker
```

API, `002_workflow.sql` migration'ını başlangıçta otomatik uygular. Kullanıcılar, arızalar ve önceki işlem geçmişi korunur; volume silmek gerekmez. Tarayıcıyı yeniledikten sonra yönetici **İş emirleri**, teknisyen **İşlerim** ekranını kullanır.

Arıza olayının `location` alanı kayıtlı cihazın konumuyla eşleşmelidir. Cihaz kimliği, kod, açıklama, önem ve RFC3339 zaman zorunludur. Beş dakikadan fazla gelecekteki olaylar reddedilir. Aynı `event_id` için ilk başarıyla kaydedilen içerik geçerlidir.

## Geliştirme ve test

```sh
go test -race ./...
go vet ./...
go build ./...
cd web
npm ci
npm run dev
```

Vite `http://localhost:5173` üzerinden `/api` isteklerini `localhost:8080` adresine yönlendirir. Backend'i doğrudan çalıştırırken `.env.example` içindeki `DATABASE_URL`, `RABBITMQ_URL`, `JWT_SECRET`, `INGEST_TOKEN`, `ADMIN_EMAIL`, `ADMIN_PASSWORD` değişkenlerini export edin. Compose servisler arası adresleri kendisi oluşturur; setup dosyası doğrudan yerel `go run` için bağlantı adresi içermez. API, worker ve simülatör ayrı komutlardır:

```sh
go run ./cmd/api
go run ./cmd/worker
go run ./cmd/simulator
```

Gerçek PostgreSQL + RabbitMQ entegrasyon testi, ana veri ve kuyruklara dokunmadan ayrı servislerle:

```sh
docker compose --profile test run --build --rm integration
docker compose --profile test stop test-postgres test-rabbitmq
```

Test; HTTP olay kabulünü, broker teslimini, eşzamanlı idempotency'yi, tekrar birleştirmesini, yönetici/teknisyen yetki ayrımını, bekleme/devir/yeniden atama/çözüm onayı akışını, kayıt görünürlüğünü ve çözüm sonrası yeni arızayla geri gönderme çakışmasını kontrol eder. Worker'ın işlem fonksiyonunu gerçek mesaj üzerinde çağırır; worker process'inin yeniden bağlanma/retry/DLQ davranışı bu testin kapsamı dışındadır. Ortam değişkenleri verilmezse entegrasyon testi açıkça SKIP olur. Gerçek Ollama modeliyle kalite değerlendirmesi otomatik testlerin kapsamında değildir.

## Operasyon

- `GET /healthz`: process canlılığı.
- `GET /readyz`: PostgreSQL erişimi ve API RabbitMQ bağlantısı. Worker canlılığını veya AI modelini doğrulamaz.
- `GET /openapi.yaml`: API sözleşmesi.
- RabbitMQ yönetimi: `http://localhost:15672`, kullanıcı `urbanops`, parola `.env` içindeki `POSTGRES_PASSWORD`.
- Kuyruklar: `urbanops.events`, `urbanops.events.retry`, `urbanops.events.dead`.
- DLQ inceleme: yönetim arayüzünde mesajın `x-error` ve `x-retry` alanlarını kontrol edin. Nedeni düzelttikten sonra aynı gövdeyi `/api/events` üzerinden aynı `event_id` ile tekrar gönderin; eski DLQ kaydını ancak başarılı işlemden sonra kaldırın. Ham DLQ mesajlarını loglara dökmeyin.
- `docker compose logs -f api worker` ile yapılandırılmış uygulama logları.
- API bağlantı kopmasında kapanır ve Compose tarafından yeniden başlatılır; worker kendi yeniden bağlanır.
- Sadece localhost portları açılır. Üretim öncesinde TLS, merkezi secret yönetimi, yedekleme/geri yükleme, paylaşımlı rate limit, token yenileme/iptal, sayfalama ve gözlemlenebilirlik tamamlanmalıdır.

## Kapsam ve sonraki adımlar

Bu sürüm bir MVP başlangıç uygulamasıdır. Redis ve sqlc henüz eklenmedi; sorgular parametreli pgx ile çalışır. Listeler en fazla 500 kayıt döndürür. Dosya yükleme, doküman versiyonlama, geçmiş çözümlerin embedding hattı, harita, ileri metrikler, refresh token, cihaz sertifikaları, DLQ yeniden işleme ekranı ve üretim altyapısı sonraki aşamalardır. Tarih aralığına göre filtreleme ve SLA raporları henüz yoktur.

Mimari ayrıntılar: [docs/architecture.md](docs/architecture.md). Testlerin bu geliştirme ortamındaki durumu: [docs/verification.md](docs/verification.md).

## Yeni personel eğitim veri seti

Her rolden üç personel, üç onaylı geçmiş çözüm ve yeni teknisyene atanmış üç tekrar senaryosu hazırlandı. Kullanım ve kaynak dökümü: [docs/training/README.md](docs/training/README.md). Hesap bilgileri `.local/training-accounts.json` içindedir. Veri seti gerçek operasyon kayıtları değildir; demo kayıtları rapor toplamlarına dahildir.

```sh
python3 scripts/seed_training.py
```

`IMPORT_TRAINING_DOCS=true` ile API başlangıcında üç araştırılmış teknik kaynak özeti model gerekmeksizin yüklenir. Bu seçenek varsayılan olarak kapalıdır; bu çalışma alanında açıldı. Kaynak metinleri binary'ye gömülüdür ve aynı içerik yeniden başlatmada çoğaltılmaz.

## Manuel arıza bildirimi

Tüm roller panelde **Arıza bildir** ekranını kullanabilir. Zorunlu alanlar: kayıtlı cihaz, hata türü/kodu, önem derecesi ve en az 10 karakterlik açıklama. Konum cihazdan otomatik alınır. Gözlem zamanı, kavşak/pano/yön gibi konum detayı ve saha notu/ilk kontroller isteğe bağlıdır. Gözlem zamanı boşsa sunucu zamanı kullanılır; gelecekteki hatalı zamanlar reddedilir. Bildiren kişi ve kayıt zamanı oturumdan/sunucudan alınır; kullanıcı başka personel adına bildirim yapamaz.

- Yeni cihaz/hata için açık arıza ve atanmamış iş emri oluşturulur; yönetici atama kuyruğunda görünür.
- Aynı cihaz ve hata kodunda açık arıza varsa yeni gözlem mevcut kayda eklenir. Tekrar sayısı ve gerekirse önem derecesi yükselir; mevcut iş emri, sorumlusu ve durumu korunur.
- Teknisyen **Bildirimlerim** ekranında kendi açtığı kayıtların durumunu izler. Bildirim açmış olmak, başka teknisyene atanmış işin detaylarına veya işlem değiştirme yetkisine erişim sağlamaz.
- Yönetim **Manuel bildirimler** ekranından tüm saha bildirimlerini görür. Arıza detayındaki **Arıza gözlemleri ve bildirenler** bölümünde ayrı açıklamalar, kaynak ve saha notları gösterilir; ilk açıklama silinmez.
- Hata türü listesinde bulunmayan sorun için özel hata kodu girilebilir. Aynı sorunda aynı kodun kullanılması tekrar analizi ve geçmiş çözüm erişimi için önemlidir.

Manuel bildirimler `POST /api/fault-reports` üzerinden aynı iş kurallarını kullanan tek PostgreSQL transaction'ında senkron kaydedilir. Cihaz olayları RabbitMQ üzerinden worker tarafından işlenmeye devam eder. İstemcinin `request_id` değeri kullanıcı kapsamında ayrılır. Aynı isteğin tekrarında mükerrer olay/audit oluşmaz; aynı anahtarla değişen içerik 409 döndürür. Kullanıcı form alanlarını değiştirirse panel yeni anahtar üretir.

`003_manual_reports.sql` mevcut olaylara varsayılan `device` kaynağı ekler. Eski kayıtlarda personel kimliği uydurulmaz; bunlar **Cihaz / otomatik** olarak gösterilir. Güncellenmiş simülatör yeni olayları `simulator` kaynağıyla gönderir. `source=manual` otomatik olay API'sinde kabul edilmez.

Güncelleme: `docker compose up --build -d api worker` ardından tarayıcıyı yenileyin. Kayıtları koruyan migration API başlangıcında otomatik uygulanır.

## Çözümün tekrar arızasında kullanılması

Canlı demo adımları: [docs/scenarios/telemetry-repeat.md](docs/scenarios/telemetry-repeat.md). İE-10 Ayhan tarafından çözülüp yönetici tarafından kapatıldı; aynı hata kodundaki İE-11 Bartu Kurnaz'a atandı. Geçmiş çözüm kaynağı `fault-10` olarak izlenebilir.

Bu çalışma alanında hazır yerel modeller için ayar:

```dotenv
OLLAMA_URL=http://host.docker.internal:11434
OLLAMA_CHAT_MODEL=qwen3:4b
OLLAMA_EMBED_MODEL=bge-m3:latest
```

Bu URL Docker içindeki API içindir. Backend'i doğrudan host üzerinde çalıştırırken `OLLAMA_URL=http://localhost:11434` kullanılır. Compose artık `OLLAMA_URL` değişkenini okur; ayar yoksa varsayılan `http://ollama:11434` servis adresidir.

Üretilen yanıt için JSON şeması, `answer` ve `citations` alanlarını ve getirilen kaynak kimliklerini sınırlar. Talimatlar geçmiş çözümdeki profil, süre, ayar ve doğrulama ayrıntılarının korunmasını ve demo etiketinin aktarılmasını ister. Modelin teknik doğruluğu yalnızca geçerli kaynak kimliği üretmesiyle kanıtlanmaz; demo çıktısı gerçek kaynak değerleriyle ayrıca karşılaştırılır.

Onaylı geçmiş çözüm araması için embedding zorunlu değildir. Seçili embedding modeline ait vektörlü belge parçası yoksa asistan embedding çağrısını atlayıp bulunan geçmiş çözümü doğrudan yanıt modeline verir. Vektör araması başarısız olsa da bulunan ilişkisel kaynaklarla yanıt üretimi denenir.
