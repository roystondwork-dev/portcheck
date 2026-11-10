# Network: IPs, devices, lookup (`net`)

`portcheck net` answers the questions that come right before or after a port problem:
what is my IP address, what else is on my network, and what is this name or address?

## Your addresses: `net`

```console
$ portcheck net
INTERFACE  ADDRESS          CLASS       MAC                NOTE
lo0        127.0.0.1/8      loopback    -
en0        192.168.1.10/24  private     00:00:5e:00:53:0a  primary
en0        fe80::1/64       link-local  00:00:5e:00:53:0a
utun3      100.64.0.5/32    shared      -                  vpn
gateway    192.168.1.1      private     -
dns        192.168.1.1      private     -
```

- One row per address of every interface that is up, with its class: `loopback`,
  `link-local`, `private` (RFC 1918 and IPv6 unique local), `shared` (carrier-grade NAT,
  common on VPNs like Tailscale), `multicast` or `public`.
- **primary** marks the address your machine uses to reach the internet. portcheck finds
  it by asking the OS which route it would use; nothing is sent.
- **vpn** and **virtual** are guesses from the interface name (`utun`, `wg`, `tailscale`;
  `docker0`, `bridge`, `vmnet`).
- The last rows are the default **gateway** (your router) and the **DNS** servers.

Everything is read from your machine. To also see your **public** address, the one the
internet sees, add `--public`:

```console
$ portcheck net --public
...
public     203.0.113.7      public      -                  from ipify
```

`--public` is the only part of `portcheck net` that contacts the internet: it makes one
HTTPS request each to `api.ipify.org` and `api6.ipify.org` ([ipify](https://www.ipify.org)).

In scripts, use `-o plain` or `--json`:

```sh
portcheck net -o plain | awk '$1 == "gateway" {print $2}'
portcheck net --json | jq -r '.data.primary.ipv4'
```

## Devices on your network: `net devices`

```console
$ portcheck net devices
IP            MAC                VENDOR                    NAME        INTERFACE  NOTE
192.168.1.1   00:00:5e:00:53:01  -                         router.lan  en0        gateway
192.168.1.10  00:00:5e:00:53:0a  -                         -           en0        this machine
192.168.1.20  b8:27:eb:00:00:20  Raspberry Pi              pi.lan      en0
192.168.1.34  da:a1:19:00:00:34  private (randomized) MAC  -           en0
```

These are the devices your machine has talked to recently: the entries of its
neighbour (ARP) cache. For each one portcheck shows:

- a **vendor hint** from the first half of the MAC address, for common makers
  (Apple, Raspberry Pi, Espressif, Google, Amazon, Sonos, TP-Link, Netgear, VMware and a
  few more). Phones and laptops usually use a **private (randomized) MAC** on Wi-Fi, so
  their vendor can't be told;
- a **name** from reverse DNS, which many home routers provide (`--no-names` skips the
  lookups);
- which device is **this machine** and which is the **gateway**.

Nothing is sent to your network except those name lookups.

### Finding the rest: `--probe`

The cache only has devices your machine talked to. `--probe` first tries a TCP connection
to a few common ports on every address of your network (22, 80, 443, 445, 548, 3389,
8008, 8009, 9100, 62078), which makes each device that's on answer and join the cache,
even if all those ports are closed:

```console
$ portcheck net devices --probe
portcheck: probing 254 hosts on 192.168.1.0/24, your local network (only probe networks you own or may test)...
```

It takes a few seconds for a typical home network. Rules:

- Only **private** (RFC 1918) and **link-local** IPv4 networks are probed; portcheck
  refuses public networks.
- At most 1024 hosts (a /22) unless you raise `--max-hosts`.
- Connections are rate-limited, and Ctrl-C stops it.

!!! warning "Only probe networks you own or are allowed to test"
    On a work, school or public network, probing may be against the rules and can set off
    security alerts. Your home network is fine.

### macOS: Local Network permission

On macOS 15 and later, the system can hide neighbour-cache entries from apps that don't
have **Local Network** permission, and `net devices` may then show only this machine,
with a hint. Allow your terminal app in **System Settings → Privacy & Security → Local
Network**, or use `--probe`: devices portcheck has contacted itself are listed.

## Look up a name or an address: `net lookup`

```console
$ portcheck net lookup example.com 192.168.1.20
QUERY         TYPE   VALUE               NOTE
example.com   A      192.0.2.80          public
example.com   AAAA   2001:db8::80        public
example.com   MX     mail.example.com    preference 10
example.com   NS     ns1.example.com
example.com   TXT    v=spf1 -all
192.168.1.20  CLASS  private             IPv4
192.168.1.20  PTR    pi.lan
192.168.1.20  LOCAL  on a local network  en0
192.168.1.20  MAC    b8:27:eb:00:00:20   Raspberry Pi
```

- For a **domain**: its A, AAAA, CNAME, MX, NS and TXT records, and the class of each
  address. Long TXT records are shortened in the table; `--json` has them whole.
- For an **address**: its class, its reverse DNS name, whether it's this machine or on one
  of your networks, and, if it's in the neighbour cache, the device's MAC and vendor.

Only your system's resolver is used, so the answers are what your machine sees,
including names from your router or VPN. Exit status 1 means a domain didn't resolve.

## Per operating system

| | macOS | Linux | Windows |
|---|---|---|---|
| Interfaces, primary address | yes | yes | yes |
| Default gateway | `route -n get default` | `/proc/net/route` | `route print` |
| DNS servers | `/etc/resolv.conf` | `/etc/resolv.conf` | not shown |
| Neighbour cache | routing socket (IPv4) | `/proc/net/arp` (IPv4) | `arp -a` (IPv4) |

On systems using `systemd-resolved`, `/etc/resolv.conf` lists the local stub resolver
(`127.0.0.53`) rather than the upstream servers. IPv6 neighbours aren't listed.
