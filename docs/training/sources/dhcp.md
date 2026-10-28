# DHCP adres edinme — kaynak özeti
Kaynak: IETF RFC 2131, Dynamic Host Configuration Protocol.
URL: https://www.rfc-editor.org/rfc/rfc2131.html
Bölümler: 3.1 Client-server interaction; 4.3.1 DHCPDISCOVER; 4.4.1 Initialization.
Erişim: 2026-10-28. Özgün Türkçe özet; tam standart metni değildir.

DHCP istemcisi adres ararken DHCPDISCOVER gönderir; sunucu DHCPOFFER ile teklif verir. İstemci DHCPREQUEST ile seçimini bildirir ve sunucu DHCPACK ile adres yapılandırmasını onaylar. Adres seçimi istemcinin bulunduğu alt ağ ve varsa relay bilgisini dikkate alır. Sunucuda kullanılabilir adres bulunmaması adres tahsisini etkiler.

UrbanOps eğitim uygulaması: DHCP_ADDRESS_FAILURE olayında mesaj akışının hangi aşamada durduğunu, istemci kimliğini, adres havuzunu ve ilgili alt ağ/relay eşleşmesini gözlemleyin. Kaynak, belirli bir üreticinin VLAN komutlarını tanımlamaz. Demo kaydındaki havuz ve relay sorunu simüle edilmiştir. Rastgele statik IP vermek yerine ölçüm, onaylı yapılandırma incelemesi ve yeni adres edinme sonucunu çözüm notuna yazın.
