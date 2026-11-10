package netinfo

// ouiVendors maps the first three bytes of a MAC address (the OUI) to its
// vendor, for a small set of common devices. Source: the IEEE Registration
// Authority's public MA-L listing (https://standards-oui.ieee.org/). It is a
// hint only: vendors own many more blocks than listed here, and phones and
// laptops often use randomised addresses instead.
var ouiVendors = map[string]string{
	// Apple
	"00:03:93": "Apple", "00:0a:95": "Apple", "00:17:f2": "Apple", "00:1b:63": "Apple",
	"00:1e:c2": "Apple", "00:25:00": "Apple", "28:cf:e9": "Apple", "3c:07:54": "Apple",
	"7c:d1:c3": "Apple", "a4:5e:60": "Apple", "ac:bc:32": "Apple", "d0:23:db": "Apple",
	"f0:18:98": "Apple",
	// Raspberry Pi
	"b8:27:eb": "Raspberry Pi", "dc:a6:32": "Raspberry Pi", "e4:5f:01": "Raspberry Pi",
	// Espressif (ESP8266/ESP32 smart-home devices)
	"24:0a:c4": "Espressif", "24:6f:28": "Espressif", "30:ae:a4": "Espressif", "a4:cf:12": "Espressif",
	// Google
	"3c:5a:b4": "Google", "54:60:09": "Google", "f4:f5:d8": "Google", "f4:f5:e8": "Google",
	// Amazon
	"44:65:0d": "Amazon", "68:37:e9": "Amazon", "74:c2:46": "Amazon", "f0:27:2d": "Amazon", "fc:65:de": "Amazon",
	// Samsung
	"00:12:fb": "Samsung", "00:16:32": "Samsung", "5c:0a:5b": "Samsung", "8c:77:12": "Samsung",
	// Sonos
	"00:0e:58": "Sonos", "5c:aa:fd": "Sonos", "94:9f:3e": "Sonos", "b8:e9:37": "Sonos",
	// Networking gear
	"00:14:6c": "Netgear", "20:4e:7f": "Netgear", "a0:40:a0": "Netgear",
	"50:c7:bf": "TP-Link", "98:de:d0": "TP-Link", "ec:08:6b": "TP-Link", "f4:f2:6d": "TP-Link",
	"24:a4:3c": "Ubiquiti", "74:83:c2": "Ubiquiti", "78:8a:20": "Ubiquiti", "80:2a:a8": "Ubiquiti", "fc:ec:da": "Ubiquiti",
	// PCs
	"00:1b:21": "Intel", "3c:a9:f4": "Intel", "a4:4e:31": "Intel",
	// Virtual machines and containers
	"00:05:69": "VMware", "00:0c:29": "VMware", "00:50:56": "VMware",
	"08:00:27": "VirtualBox",
	"00:15:5d": "Hyper-V",
	"52:54:00": "QEMU/KVM",
	"02:42:ac": "Docker",
}

// Vendor returns the vendor hint for a normalised MAC address: a known vendor,
// "private (randomized) MAC" for a locally administered address, or "".
func Vendor(mac string) string {
	if len(mac) < 8 {
		return ""
	}
	if v, ok := ouiVendors[mac[:8]]; ok {
		return v
	}
	if RandomizedMAC(mac) {
		return "private (randomized) MAC"
	}
	return ""
}
