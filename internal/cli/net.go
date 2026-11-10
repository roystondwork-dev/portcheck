package cli

import (
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"time"

	"github.com/spf13/cobra"

	"github.com/roystondwork-dev/portcheck/internal/output"
	"github.com/roystondwork-dev/portcheck/pkg/netinfo"
	"github.com/roystondwork-dev/portcheck/pkg/portspec"
	"github.com/roystondwork-dev/portcheck/pkg/scan"
)

func (c *cli) newNetCmd() *cobra.Command {
	var public bool
	ip := func(cmd *cobra.Command, _ []string) error { return c.runNetIP(cmd, public) }
	cmd := &cobra.Command{
		Use:   "net",
		Short: "Show this machine's IP addresses, gateway and DNS servers",
		Long: `Show this machine's network interfaces and addresses, the primary address
(the one used to reach the internet), the default gateway and the DNS servers.

Everything is read locally. --public also asks ipify (https://www.ipify.org)
for the network's public address: the only part that contacts the internet.`,
		Example: `  portcheck net
  portcheck net --public
  portcheck net -o plain | awk '$1 == "gateway" {print $2}'
  portcheck net devices
  portcheck net lookup example.com 192.168.1.1`,
		Args: usageArgs(cobra.NoArgs),
		RunE: ip,
	}
	cmd.Flags().BoolVar(&public, "public", false, "also show the public address, from ipify (contacts the internet)")
	ipCmd := &cobra.Command{
		Use:   "ip",
		Short: "Show this machine's IP addresses (the same as 'portcheck net')",
		Args:  usageArgs(cobra.NoArgs),
		RunE:  ip,
	}
	ipCmd.Flags().BoolVar(&public, "public", false, "also show the public address, from ipify (contacts the internet)")
	cmd.AddCommand(ipCmd, c.newNetDevicesCmd(), c.newNetLookupCmd())
	return cmd
}

func (c *cli) runNetIP(cmd *cobra.Command, public bool) error {
	id, err := netinfo.Collect(cmd.Context(), c.app.Net, netinfo.Options{Public: public, Get: c.app.NetGet})
	if err != nil {
		return err
	}
	c.warn(id.Warnings)
	return c.printer.Print(output.NetDoc(id))
}

// warn prints non-fatal problems on stderr.
func (c *cli) warn(warnings []string) {
	for _, w := range warnings {
		fmt.Fprintf(c.app.Stderr, "portcheck: %s\n", w)
	}
}

type netDevicesFlags struct {
	noNames  bool
	probe    bool
	maxHosts int
}

func (c *cli) newNetDevicesCmd() *cobra.Command {
	var f netDevicesFlags
	cmd := &cobra.Command{
		Use:   "devices",
		Short: "List the devices on your local network",
		Long: `List the devices on your local network that this machine knows about: the
entries of its neighbour (ARP) cache, with a vendor hint from the MAC address and
a name from reverse DNS. Nothing is sent to the network except those DNS
queries (--no-names skips them).

--probe first tries a TCP connection to a few common ports on every address of
the primary interface's network, so that devices answer and join the cache. It
only probes private (RFC 1918) and link-local IPv4 networks of up to
--max-hosts hosts. Only probe networks you own or are allowed to test.`,
		Example: `  portcheck net devices
  portcheck net devices --probe
  portcheck net devices --no-names --json`,
		Args: usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error { return c.runNetDevices(cmd, f) },
	}
	fl := cmd.Flags()
	fl.BoolVar(&f.noNames, "no-names", false, "don't look up device names (reverse DNS)")
	fl.BoolVar(&f.probe, "probe", false, "probe the local network first, to find devices not in the cache")
	fl.IntVar(&f.maxHosts, "max-hosts", netinfo.DefaultProbeHosts, "with --probe: the largest network to probe")
	return cmd
}

func (c *cli) runNetDevices(cmd *cobra.Command, f netDevicesFlags) error {
	ctx := cmd.Context()
	probed := ""
	var open []netip.Addr
	if f.probe {
		p, hosts, err := c.probe(cmd, f.maxHosts)
		if err != nil {
			return err
		}
		probed, open = p, hosts
	}
	opts := netinfo.DeviceOptions{}
	if !f.noNames {
		opts.Names = c.app.DNS
	}
	devs, warnings, err := netinfo.Devices(ctx, c.app.Net, opts)
	if err != nil {
		return err
	}
	// Hosts that answered but aren't in the cache (e.g. it was flushed meanwhile).
	for _, a := range open {
		if !slices.ContainsFunc(devs, func(d netinfo.Device) bool { return d.IP == a }) {
			devs = append(devs, netinfo.Device{IP: a})
		}
	}
	slices.SortFunc(devs, func(a, b netinfo.Device) int { return a.IP.Compare(b.IP) })
	c.warn(warnings)
	return c.printer.Print(output.DevicesDoc(devs, probed, warnings))
}

// probe tries a few TCP ports on every host of the primary network and returns
// the network and the hosts with an open port.
func (c *cli) probe(cmd *cobra.Command, maxHosts int) (string, []netip.Addr, error) {
	ifaces, err := c.app.Net.Interfaces()
	if err != nil {
		return "", nil, err
	}
	primary, _ := c.app.Net.Primary("udp4")
	prefix, ok := netinfo.PrimaryPrefix(ifaces, primary)
	if !ok {
		return "", nil, &ExitError{Code: ExitRuntime, Err: errors.New("no IPv4 network to probe: this machine has no IPv4 route")}
	}
	hosts, err := netinfo.ProbeTargets(prefix, maxHosts)
	if err != nil {
		return "", nil, usageError("%v", err)
	}
	fmt.Fprintf(c.app.Stderr, "portcheck: probing %d hosts on %s, your local network (only probe networks you own or may test)...\n", len(hosts), prefix)
	targets := make([]scan.Target, len(hosts))
	for i, h := range hosts {
		targets[i] = scan.Target{Host: h.String(), Addr: h}
	}
	opts := scan.Options{Concurrency: 64, Timeout: 600 * time.Millisecond, Rate: 400}
	var open []netip.Addr
	for r := range c.app.Scan(cmd.Context(), targets, portspec.MustParse(netinfo.ProbePorts), opts) {
		if r.State == scan.Open && !slices.Contains(open, r.Target.Addr) {
			open = append(open, r.Target.Addr)
		}
	}
	if err := cmd.Context().Err(); err != nil {
		return "", nil, err //nolint:wrapcheck // interrupted
	}
	return prefix.String(), open, nil
}

func (c *cli) newNetLookupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "lookup <domain|ip>...",
		Short: "Look up DNS records of a domain, or what an IP address is",
		Long: `Look up a domain's A, AAAA, CNAME, MX, NS and TXT records, or an IP address's
class (private, public, ...), reverse DNS name, and whether it is this machine or
on one of its networks (with the device's MAC address and vendor, if known).

Only the system resolver is used. Exit status 1 means a domain didn't resolve.`,
		Example: `  portcheck net lookup example.com
  portcheck net lookup 192.168.1.1 1.1.1.1
  portcheck net lookup example.com --json`,
		Args: usageArgs(cobra.MinimumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error { return c.runNetLookup(cmd, args) },
	}
}

func (c *cli) runNetLookup(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	local := netinfo.Local{}
	local.Interfaces, _ = c.app.Net.Interfaces() // best effort: only for "this machine" and MACs
	local.Neighbors, _ = c.app.Net.Neighbors(ctx)
	results := make([]netinfo.LookupResult, len(args))
	failed := false
	for i, q := range args {
		results[i] = netinfo.Lookup(ctx, c.app.DNS, local, q)
		if results[i].Err != "" {
			fmt.Fprintf(c.app.Stderr, "portcheck: %s\n", results[i].Err)
			failed = true
		}
	}
	if err := c.printer.Print(output.LookupDoc(results)); err != nil {
		return err
	}
	if failed {
		return negative()
	}
	return nil
}
