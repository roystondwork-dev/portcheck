package output

import (
	"net/netip"
	"strings"

	"github.com/roystondwork-dev/portcheck/pkg/netinfo"
)

// NetAddressJSON is an interface address.
type NetAddressJSON struct {
	Address string `json:"address"` // with prefix length, e.g. "192.0.2.10/24"
	Class   string `json:"class"`
	Version string `json:"version"`
}

// NetInterfaceJSON is a network interface that is up.
type NetInterfaceJSON struct {
	Name      string           `json:"name"`
	MAC       string           `json:"mac,omitempty"`
	MTU       int              `json:"mtu"`
	Kind      string           `json:"kind"`
	Addresses []NetAddressJSON `json:"addresses"`
}

// NetPrimaryJSON is the route to the internet.
type NetPrimaryJSON struct {
	IPv4      string `json:"ipv4,omitempty"`
	IPv6      string `json:"ipv6,omitempty"`
	Interface string `json:"interface,omitempty"`
}

// NetJSON is the payload of `portcheck net`.
type NetJSON struct {
	Interfaces []NetInterfaceJSON `json:"interfaces"`
	Primary    NetPrimaryJSON     `json:"primary"`
	Gateways   []string           `json:"gateways"`
	DNS        []string           `json:"dns"`
	// Public is set only with --public.
	Public   []string `json:"public,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

func addrStrings(as []netip.Addr) []string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = a.String()
	}
	return out
}

func validString(a netip.Addr) string {
	if !a.IsValid() {
		return ""
	}
	return a.String()
}

// NetDoc renders `portcheck net`: one row per interface address, then the
// gateways, DNS servers and public addresses, so a script can grep for them.
func NetDoc(id netinfo.Identity) Document {
	data := NetJSON{
		Interfaces: make([]NetInterfaceJSON, len(id.Interfaces)),
		Primary:    NetPrimaryJSON{IPv4: validString(id.Primary4), IPv6: validString(id.Primary6), Interface: id.PrimaryInterface},
		Gateways:   addrStrings(id.Gateways),
		DNS:        addrStrings(id.DNS),
		Warnings:   id.Warnings,
	}
	if id.Public != nil {
		data.Public = addrStrings(id.Public)
	}
	d := Document{
		Kind:    "net",
		Headers: []string{"INTERFACE", "ADDRESS", "CLASS", "MAC", "NOTE"},
		Empty:   "No network interfaces are up.",
	}
	for i, ni := range id.Interfaces {
		j := NetInterfaceJSON{Name: ni.Name, MAC: ni.MAC, MTU: ni.MTU, Kind: ni.Kind, Addresses: []NetAddressJSON{}}
		for _, p := range ni.Prefixes {
			class := netinfo.Classify(p.Addr())
			j.Addresses = append(j.Addresses, NetAddressJSON{Address: p.String(), Class: class, Version: netinfo.Version(p.Addr())})
			var notes []string
			if p.Addr() == id.Primary4 || p.Addr() == id.Primary6 {
				notes = append(notes, "primary")
			}
			if ni.Kind == netinfo.KindVPN || ni.Kind == netinfo.KindVirtual {
				notes = append(notes, ni.Kind)
			}
			d.Rows = append(d.Rows, []Cell{C(ni.Name), C(p.String()), classCell(class), C(dash(ni.MAC)), T(strings.Join(notes, ", "), ToneInfo)})
		}
		data.Interfaces[i] = j
	}
	extra := func(name string, as []netip.Addr, note string) {
		for _, a := range as {
			d.Rows = append(d.Rows, []Cell{T(name, ToneMuted), C(a.String()), classCell(netinfo.Classify(a)), C("-"), T(note, ToneInfo)})
		}
	}
	extra("gateway", id.Gateways, "")
	extra("dns", id.DNS, "")
	extra("public", id.Public, "from ipify")
	d.Data = data
	return d
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func classCell(class string) Cell {
	switch class {
	case netinfo.ClassPublic:
		return T(class, ToneWarn)
	case netinfo.ClassPrivate, netinfo.ClassLinkLocal:
		return T(class, ToneGood)
	default:
		return T(class, ToneMuted)
	}
}

// DeviceJSON is a device on a local network.
type DeviceJSON struct {
	IP        string `json:"ip"`
	MAC       string `json:"mac,omitempty"`
	Vendor    string `json:"vendor,omitempty"`
	Name      string `json:"name,omitempty"`
	Interface string `json:"interface,omitempty"`
	Self      bool   `json:"self,omitempty"`
	Gateway   bool   `json:"gateway,omitempty"`
}

// DevicesJSON is the payload of `portcheck net devices`.
type DevicesJSON struct {
	Devices []DeviceJSON `json:"devices"`
	// Probed is the network probed with --probe, if any.
	Probed   string   `json:"probed,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// DevicesDoc renders `portcheck net devices`.
func DevicesDoc(devs []netinfo.Device, probed string, warnings []string) Document {
	data := DevicesJSON{Devices: make([]DeviceJSON, len(devs)), Probed: probed, Warnings: warnings}
	d := Document{
		Kind:    "devices",
		Headers: []string{"IP", "MAC", "VENDOR", "NAME", "INTERFACE", "NOTE"},
		Empty:   "No devices in the neighbour cache. Try --probe to look for them.",
	}
	for i, dev := range devs {
		data.Devices[i] = DeviceJSON{
			IP: dev.IP.String(), MAC: dev.MAC, Vendor: dev.Vendor, Name: dev.Name,
			Interface: dev.Interface, Self: dev.Self, Gateway: dev.Gateway,
		}
		var notes []string
		if dev.Self {
			notes = append(notes, "this machine")
		}
		if dev.Gateway {
			notes = append(notes, "gateway")
		}
		d.Rows = append(d.Rows, []Cell{
			C(dev.IP.String()), C(dash(dev.MAC)), C(dash(dev.Vendor)), C(dash(dev.Name)), C(dash(dev.Interface)),
			T(strings.Join(notes, ", "), ToneInfo),
		})
	}
	d.Data = data
	return d
}

// RecordJSON is a fact about a looked-up name or address.
type RecordJSON struct {
	Type  string `json:"type"`
	Value string `json:"value"`
	Note  string `json:"note,omitempty"`
}

// LookupResultJSON is what `net lookup` found for one query.
type LookupResultJSON struct {
	Query   string       `json:"query"`
	Kind    string       `json:"kind"`
	Records []RecordJSON `json:"records"`
	Error   string       `json:"error,omitempty"`
}

// LookupJSON is the payload of `portcheck net lookup`.
type LookupJSON struct {
	Results []LookupResultJSON `json:"results"`
}

// maxRecordWidth bounds long values (TXT records) in tables; JSON has them whole.
const maxRecordWidth = 72

// LookupDoc renders `portcheck net lookup`: one row per record.
func LookupDoc(results []netinfo.LookupResult) Document {
	data := LookupJSON{Results: make([]LookupResultJSON, len(results))}
	d := Document{Kind: "lookup", Headers: []string{"QUERY", "TYPE", "VALUE", "NOTE"}, Empty: "Nothing found."}
	for i, r := range results {
		j := LookupResultJSON{Query: r.Query, Kind: r.Kind, Records: make([]RecordJSON, len(r.Records)), Error: r.Err}
		for k, rec := range r.Records {
			j.Records[k] = RecordJSON(rec)
			v := rec.Value
			if len([]rune(v)) > maxRecordWidth {
				v = string([]rune(v)[:maxRecordWidth-3]) + "..."
			}
			value, note := C(v), C(rec.Note)
			switch rec.Type {
			case "A", "AAAA":
				note = classCell(rec.Note)
			case "CLASS":
				value = classCell(rec.Value)
			}
			d.Rows = append(d.Rows, []Cell{C(r.Query), T(rec.Type, ToneInfo), value, note})
		}
		data.Results[i] = j
	}
	d.Data = data
	return d
}
