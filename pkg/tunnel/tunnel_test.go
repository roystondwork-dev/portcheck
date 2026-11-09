package tunnel

import "testing"

func TestParse(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		cmdline string
		port    uint16
		want    string // "" for no tunnel
	}{
		// SSH local forwards.
		{"ssh -L 5432:db.internal:5432 ops@bastion", 5432, "SSH tunnel to db.internal:5432 via ops@bastion"},
		{"/usr/bin/ssh -N -L 127.0.0.1:15432:db:5432 bastion", 15432, "SSH tunnel to db:5432 via bastion"},
		{"ssh -fNL8080:localhost:80 -l deploy web1", 8080, "SSH tunnel to localhost:80 via deploy@web1"},
		{"ssh -L [::1]:9000:[fd00::5]:9000 -p 2222 me@host", 9000, "SSH tunnel to [fd00::5]:9000 via me@host"},
		{"ssh -L 1:a:1 -L 2:b:2 host", 2, "SSH tunnel to b:2 via host"},
		{"ssh -L 2:a:2 -L 2:b:2 host", 2, "SSH tunnel to a:2 via host"},
		{`C:\Windows\System32\OpenSSH\ssh.exe -L 3389:pc:3389 jump`, 3389, "SSH tunnel to pc:3389 via jump"},
		{"ssh -i key.pem -o ServerAliveInterval=30 -L 6379:cache:6379 user@h extra-cmd", 6379, "SSH tunnel to cache:6379 via user@h"},
		// SSH SOCKS proxies.
		{"ssh -D 1080 -N proxyhost", 1080, "SSH SOCKS proxy via proxyhost"},
		{"ssh -D localhost:1080 -l u proxyhost", 1080, "SSH SOCKS proxy via u@proxyhost"},
		// Port-forwards.
		{"kubectl port-forward svc/web 8080:80", 8080, "port-forward to svc/web:80"},
		{"kubectl -n prod port-forward deploy/api 9090:http", 9090, "port-forward to deploy/api:http in namespace prod"},
		{"kubectl port-forward --namespace=staging pod/x 5000 --address 0.0.0.0", 5000, "port-forward to pod/x:5000 in namespace staging"},
		{"oc port-forward -nqa svc/db 15432:5432 16379:6379", 16379, "port-forward to svc/db:6379 in namespace qa"},
		{"kubectl --context kind-dev port-forward svc/web 3000:80", 3000, "port-forward to svc/web:80"},
		// Not a tunnel on this port, or not a tunnel at all.
		{"ssh -L 5432:db:5432 bastion", 5433, ""},
		{"ssh -L 5432:db:5432", 5432, ""},             // no destination
		{"ssh -L /tmp/s.sock:db:5432 host", 5432, ""}, // unix socket
		{"ssh -L 5432:/run/pg.sock host", 5432, ""},   // unix socket target
		{"ssh -L 5432:db:notaport host", 5432, ""},
		{"ssh -R 8080:localhost:80 host", 8080, ""}, // remote forward listens remotely
		{"ssh -D 1080 proxy", 1081, ""},
		{"ssh -L", 5432, ""},
		{"ssh - host", 22, ""},
		{"kubectl port-forward svc/web 8080:80", 8081, ""},
		{"kubectl get pods", 8080, ""},
		{"kubectl port-forward svc/web", 8080, ""},
		{"kubectl port-forward svc/web 8080:", 8080, ""},
		{"kubectl -n", 8080, ""},
		{"node server.js --port 3000", 3000, ""},
		{"", 3000, ""},
	} {
		got, ok := Parse(tc.cmdline, tc.port)
		if (tc.want != "") != ok || (ok && got.String() != tc.want) {
			t.Errorf("Parse(%q, %d) = %q, %v; want %q", tc.cmdline, tc.port, got.String(), ok, tc.want)
		}
	}
}

func TestKinds(t *testing.T) {
	t.Parallel()
	if tn, _ := Parse("ssh -L 1:a:1 h", 1); tn.Kind != SSH || tn.Target != "a:1" || tn.Via != "h" {
		t.Errorf("ssh = %+v", tn)
	}
	if tn, _ := Parse("ssh -D 9 h", 9); tn.Kind != SSHSocks || tn.Target != "" {
		t.Errorf("socks = %+v", tn)
	}
	if tn, _ := Parse("kubectl port-forward svc/a 1:2", 1); tn.Kind != PortForward || tn.Via != "" {
		t.Errorf("port-forward = %+v", tn)
	}
}
