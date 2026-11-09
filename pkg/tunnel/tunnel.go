// Package tunnel recognises processes that listen on a port only to forward it
// somewhere else: SSH local forwards (ssh -L), SSH SOCKS proxies (ssh -D) and
// Kubernetes port-forwards (kubectl/oc port-forward). It reads the process's
// command line; ports forwarded through an SSH config file (LocalForward) are
// not visible there and are not recognised.
package tunnel

import (
	"path/filepath"
	"strconv"
	"strings"
)

// Kind is what a tunnel forwards through.
type Kind string

// The kinds of tunnel.
const (
	SSH         Kind = "ssh"          // ssh -L
	SSHSocks    Kind = "ssh-socks"    // ssh -D
	PortForward Kind = "port-forward" // kubectl/oc port-forward
)

// Tunnel describes where a forwarded port leads.
type Tunnel struct {
	Kind Kind
	// Target is where connections go: "db.internal:5432", or for a
	// port-forward "svc/web:80". Empty for a SOCKS proxy.
	Target string
	// Via is the SSH destination ("ops@bastion") or the Kubernetes namespace.
	Via string
}

// String describes the tunnel, e.g. "SSH tunnel to db.internal:5432 via
// ops@bastion" or "kubectl port-forward to svc/web:80 in namespace prod".
func (t Tunnel) String() string {
	switch t.Kind {
	case SSHSocks:
		return "SSH SOCKS proxy via " + t.Via
	case PortForward:
		s := "port-forward to " + t.Target
		if t.Via != "" {
			s += " in namespace " + t.Via
		}
		return s
	default:
		return "SSH tunnel to " + t.Target + " via " + t.Via
	}
}

// Parse reports the tunnel that cmdline (a process's command line, arguments
// separated by spaces) runs on port, if any.
func Parse(cmdline string, port uint16) (Tunnel, bool) {
	args := strings.Fields(cmdline)
	if len(args) == 0 {
		return Tunnel{}, false
	}
	switch strings.TrimSuffix(strings.ToLower(filepath.Base(strings.ReplaceAll(args[0], `\`, "/"))), ".exe") {
	case "ssh":
		return parseSSH(args[1:], port)
	case "kubectl", "oc":
		return parsePortForward(args[1:], port)
	default:
		return Tunnel{}, false
	}
}

// sshArgOptions are the ssh options that take an argument.
const sshArgOptions = "BbcDEeFIiJLlmOoPpQRSWw"

func parseSSH(args []string, port uint16) (Tunnel, bool) {
	var dest, user string
	var found *Tunnel
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") || len(a) < 2 {
			if dest == "" {
				dest = a
			}
			continue
		}
		// Options can be combined (-fNL ...); only the last one may take an argument.
		for j := 1; j < len(a); j++ {
			opt := a[j]
			if !strings.ContainsRune(sshArgOptions, rune(opt)) {
				continue
			}
			val := a[j+1:]
			if val == "" && i+1 < len(args) {
				i++
				val = args[i]
			}
			switch opt {
			case 'L':
				if target, ok := localForward(val, port); ok && found == nil {
					found = &Tunnel{Kind: SSH, Target: target}
				}
			case 'D':
				if listenPort(val) == port && found == nil {
					found = &Tunnel{Kind: SSHSocks}
				}
			case 'l':
				user = val
			}
			break
		}
	}
	if found == nil || dest == "" {
		return Tunnel{}, false
	}
	found.Via = dest
	if user != "" && !strings.Contains(dest, "@") {
		found.Via = user + "@" + dest
	}
	return *found, true
}

// localForward parses an -L spec, "[bind:]port:host:hostport", and returns
// "host:hostport" when it listens on port. Specs with a Unix socket on
// either side are ignored.
func localForward(spec string, port uint16) (string, bool) {
	parts := splitSpec(spec)
	if len(parts) == 4 {
		parts = parts[1:] // drop the bind address
	}
	if len(parts) != 3 || parsePort(parts[0]) != port || parsePort(parts[2]) == 0 {
		return "", false
	}
	host := parts[1]
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return host + ":" + parts[2], true
}

// listenPort is the port of a -D spec, "[bind:]port".
func listenPort(spec string) uint16 {
	parts := splitSpec(spec)
	return parsePort(parts[len(parts)-1])
}

// splitSpec splits a forwarding spec at colons, keeping [IPv6] addresses whole.
func splitSpec(spec string) []string {
	var parts []string
	var cur strings.Builder
	inBrackets := false
	for _, r := range spec {
		switch {
		case r == '[':
			inBrackets = true
		case r == ']':
			inBrackets = false
		case r == ':' && !inBrackets:
			parts = append(parts, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	return append(parts, cur.String())
}

func parsePort(s string) uint16 {
	n, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		return 0
	}
	return uint16(n)
}

// kubectlArgFlags are the global and port-forward flags that take a value
// when written without "=".
var kubectlArgFlags = map[string]bool{
	"-n": true, "--namespace": true, "--address": true, "--context": true, "--kubeconfig": true,
	"--cluster": true, "--user": true, "-s": true, "--server": true, "--pod-running-timeout": true,
	"-l": true, "--selector": true,
}

func parsePortForward(args []string, port uint16) (Tunnel, bool) {
	var ns string
	var positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		name, val, hasVal := strings.Cut(a, "=")
		switch {
		case name == "-n" || name == "--namespace":
			if !hasVal && i+1 < len(args) {
				i++
				val = args[i]
			}
			ns = val
		case strings.HasPrefix(a, "-n") && len(a) > 2 && !strings.HasPrefix(a, "--"):
			ns = a[2:]
		case strings.HasPrefix(a, "-"):
			if !hasVal && kubectlArgFlags[name] {
				i++
			}
		default:
			positional = append(positional, a)
		}
	}
	// port-forward TYPE/NAME [LOCAL:]REMOTE...
	if len(positional) < 3 || positional[0] != "port-forward" {
		return Tunnel{}, false
	}
	resource := positional[1]
	for _, m := range positional[2:] {
		local, remote, mapped := strings.Cut(m, ":")
		if !mapped {
			remote = local
		}
		if parsePort(local) == port && remote != "" {
			return Tunnel{Kind: PortForward, Target: resource + ":" + remote, Via: ns}, true
		}
	}
	return Tunnel{}, false
}
