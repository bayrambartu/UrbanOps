# Yeni personel için arıza öğrenme deneyi

Bu veri seti gerçek arıza ölçümü değildir. Dokuz kurgu personel, üç sanal cihaz, yönetici tarafından onaylanmış üç çözüm ve yeni personele atanmış üç tekrar senaryosu içerir. Var olan hesaplar ve parolalar değiştirilmez.

## Hesaplar

Her rolden üç kişi vardır. Yönetici: Ayşe Demir, Murat Aydın, Selin Kaya. Operatör: Deniz Arslan, Ece Yılmaz, Can Koç. Teknisyen: kıdemli Emre Şahin, kıdemli Zeynep Aksoy ve yeni başlayan Ali Yıldız.

Giriş bilgileri `.local/training-accounts.json` ve bu oturumda oluşturulan `.local/training-accounts.md` dosyalarında saklanır; sürüm kontrolüne alınmaz. Yeni personel hesabı: `ali.yildiz@urbanops.local`. Parola dosyadaki bu satırdan okunmalıdır.

## Deney akışı

1. Ali Yıldız hesabıyla giriş yapın ve **İşlerim** ekranını açın. Üç atanmış iş görünmeli.
2. `CONNECTION_TIMEOUT` işini açın. Atama notunda yöneticinin kontrol planını okuyun.
3. **Onaylı geçmiş çözümler** bölümündeki eski işi genişletin. Kıdemli teknisyenin gözlemini, kök neden hipotezini, müdahalesini ve doğrulamasını okuyun. Yönetici onayını ayrı değerlendirin.
4. **Bilgi bankası** üzerinden TCP, Ethernet ve DHCP kaynak özetlerini okuyun; bağlantıdan özgün belgeye gidin. Vektör indeksi bekleyen metinler de okunabilir.
5. **Kaynaklı öneri getir** seçeneğini deneyin. Ollama kapalıysa doğrudan kaynak görünümü gelir; bu bir LLM yanıtı olarak gösterilmez. Model hazırsa kaynak kimlikleri doğrulanan öneri üretilebilir.
6. Müsait olduğunuzda **İşe başla** deyin. Engel varsa gerekçeyle beklemeye alın. Çözüm notuna gözlem → hipotez → kontrol → sonuç yazın; eski çözümü aynen kopyalamayın.
7. İlgili yönetici hesabıyla çözümü inceleyin, onaylayın veya gerekçeyle geri gönderin.

| Senaryo | Eski kayıttan öğrenilecek şey | Yeni olayda aranacak kanıt |
|---|---|---|
| CONNECTION_TIMEOUT | Link flap ve paket kaybı ile TCP belirtilerini ayırma | Güncel kayıp/bağlantı sayaçları; kabloyu neden suçladığının kanıtı |
| LINK_CRC_ERRORS | CRC toplamı yerine zaman içindeki artışı inceleme | Sayaç delta ve gözlem aralığı; iki uç speed/duplex durumu |
| DHCP_ADDRESS_FAILURE | DORA akışının kesildiği adımı belirleme | OFFER/ACK görülüyor mu, havuz ve relay/alt ağ eşleşmesi nasıl? |

Başarı ölçütü: Yeni personel hangi geçmiş kaydı kullandığını, hangi bulgunun yeni olayda doğrulandığını ve hangi hipotezin elendiğini açıklayabilmeli. Sistem cihaz üzerinde otomatik işlem yapmaz. Bu deney operasyonel verimlilik artışını tek başına kanıtlamaz.

## Kaynaklar ve aktarım

- [Cisco Ethernet sorun giderme](https://www.cisco.com/c/en/us/support/docs/switches/catalyst-6500-series-switches/12027-53.html): fiziksel katman, CRC, bağlantı durumu ve speed/duplex.
- [IETF RFC 6298](https://www.rfc-editor.org/rfc/rfc6298.html): TCP RTO'nun protokol anlamı.
- [IETF RFC 2131](https://www.rfc-editor.org/rfc/rfc2131.html): DHCP mesaj akışı ve adres tahsisi.

Tam metinler kopyalanmadı; erişim tarihi, ilgili bölümler ve URL bilgisiyle kısa özgün Türkçe özetler hazırlandı. Belgeler trafik sinyalizasyon kontrol cihazına özgü üretici talimatı değildir. Eğitimdeki RCA ve sayısal ölçümler simülasyondur.

Personel ve iş akışını yeniden oluşturmak/devam ettirmek için:

```sh
python3 scripts/seed_training.py
```

Komut sabit olay kimlikleriyle çalışır. Tamamlanan eğitim geçmişini ve mevcut yeni personel işlerinin ilerlemesini değiştirmez. Bir koşu bootstrap + 9 personel için 10 giriş yapar; art arda çalıştırmalarda giriş sınırı nedeniyle bir dakika beklemek gerekebilir. Kimlik bilgisi dosyası kaybolmuşsa mevcut hesap parolalarını sessizce sıfırlamaz.

Kaynak metinlerini Ollama olmadan API başlangıcında yüklemek için `.env` dosyasına `IMPORT_TRAINING_DOCS=true` ekleyip API'yi yeniden derleyin/başlatın. Bu oturumda seçenek açıldı. Aynı başlık ve içerik tekrar eklenmez. Bu işlem personel/iş üretmez; yalnızca kaynak metinlerini yükler. Model hazır olduğunda belgeleri panelden **Yeniden indeksle** ile vektör aramasına açın.
