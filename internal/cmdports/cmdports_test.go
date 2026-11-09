package cmdports

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func ex(ports ...uint16) []Candidate {
	out := make([]Candidate, len(ports))
	for i, p := range ports {
		out[i] = Candidate{p, Explicit}
	}
	return out
}

func def(p uint16) []Candidate { return []Candidate{{p, Default}} }

func TestPorts(t *testing.T) {
	t.Parallel()
	scripts := map[string]string{
		"dev":     "vite --port 3001",
		"start":   "node server.js",
		"serve":   "PORT=4000 next start",
		"web":     "next dev",
		"loop":    "npm run loop",
		"preview": "vite preview",
		"docs":    "mkdocs serve -a 127.0.0.1:8100",
	}
	lookup := func(name string) string { return scripts[name] }
	for _, tc := range []struct {
		cmd  string
		want []Candidate
	}{
		// Nothing to find.
		{"", nil},
		{"false", nil},
		{"git push origin main:main", nil},
		{"make -j8 build", nil},
		{"mkdir -p build", nil},
		{"FOO=bar", nil},
		{"=3000 vite", nil},
		{"1PORT=3000 vite", nil}, // not an assignment: "1PORT=3000" is the program
		{"PORT=abc vite", def(5173)},
		{"docker run -p 80 nginx", nil},
		{"docker run nginx", nil},
		{"docker", nil},
		{"python3", nil},
		{"node", nil},
		{"sudo", nil},

		// Explicit flags and values.
		{"vite --port 3000", ex(3000)},
		{"vite --port=3000", ex(3000)},
		{"rails s -p 4000", ex(4000)},
		{"rails s -p4000", ex(4000)},
		{"gunicorn -b 0.0.0.0:8001 app:app", ex(8001)},
		{"hugo server --bind 0.0.0.0 --port 1400", ex(1400)},
		{"caddy run --listen :8443", ex(8443)},
		{"php -S localhost:8080", ex(8080)},
		{"java -jar app.jar --server.port=9090", ex(9090)},
		{"vite --port 70000", nil}, // out of range, and 70000 isn't a subcommand
		{"vite --port 0", nil},
		{"vite --port", def(5173)},
		{"PORT=3005 node server.js", ex(3005)},
		{"VITE_PORT=3006 vite", ex(3006)},
		{"FOO=1 PORT=3007 next dev", ex(3007)},
		{"python manage.py runserver 0.0.0.0:8001", ex(8001)},
		{"python3 manage.py runserver 8002", ex(8002)},
		{"python3 -m http.server 8003", ex(8003)},
		{"python -u -m http.server", def(8000)},
		{"python -m SimpleHTTPServer 8004", ex(8004)},
		{"some-server :9000", ex(9000)},
		{"some-server [::]:9001", ex(9001)},
		{"docker run -p 8080:80 nginx", ex(8080)},
		{"docker run -p 127.0.0.1:8081:80/tcp nginx", ex(8081)},
		{"docker run --publish=8082:80 nginx", ex(8082)},
		{"docker run -p8083:80 nginx", ex(8083)},
		{"podman run -d -p 8084:80 -p 8085:443 nginx", ex(8084, 8085)},
		{"kubectl port-forward svc/web 8086:80", ex(8086)},
		{"kubectl port-forward pod/web 8087", ex(8087)},
		{"npx serve -l 5000", ex(5000)},

		// Defaults of known tools.
		{"vite", def(5173)},
		{"vite dev", def(5173)},
		{"vite preview", def(4173)},
		{"vite build", nil},
		{"./node_modules/.bin/vite", def(5173)},
		{"npx next dev", def(3000)},
		{"npx -y next start", def(3000)},
		{"pnpm exec vite", def(5173)},
		{"bunx --bun vite", def(5173)},
		{"node node_modules/.bin/vite", def(5173)},
		{"node --inspect node_modules/vite/bin/vite.js", def(5173)},
		{"react-scripts start", def(3000)},
		{"bin/rails server", def(3000)},
		{"bundle exec rails s", def(3000)},
		{"flask run", def(5000)},
		{"flask --app hello run", def(5000)},
		{"python manage.py runserver", def(8000)},
		{"uv run uvicorn main:app --reload", def(8000)},
		{"poetry run gunicorn app:app", def(8000)},
		{"fastapi dev main.py", def(8000)},
		{"hugo server", def(1313)},
		{"bundle exec jekyll serve", def(4000)},
		{"astro dev", def(4321)},
		{"ng serve", def(4200)},
		{"gatsby develop", def(8000)},
		{"php artisan serve", def(8000)},
		{"mkdocs serve", def(8000)},
		{"npx serve", def(3000)},
		{"nuxi dev", def(3000)},
		{"deno run -A npm:vite", def(5173)},
		{"sudo -E env FOO=1 vite", def(5173)},
		{"time nohup vite", def(5173)},
		{"dotenv -- next dev", def(3000)},

		// Package scripts.
		{"npm run dev", ex(3001)},
		{"npm run-script dev", ex(3001)},
		{"pnpm dev", ex(3001)},
		{"yarn dev", ex(3001)},
		{"bun run dev", ex(3001)},
		{"npm start", def(3000)},
		{"npm run serve", ex(4000)},
		{"yarn web", def(3000)},
		{"npm run preview", def(4173)},
		{"pnpm run docs", ex(8100)},
		{"npm run loop", nil},
		{"npm run missing", nil},
		{"npm run", nil},
		{"npm install", nil},
		{"yarn add vite", nil},
		{"npm", nil},
		{"npm test", nil},

		// Clients connect to ports; they don't listen on them.
		{"ssh -p 2222 host", nil},
		{"psql -h localhost -p 5432", nil},
		{"curl localhost:3000", nil},
		{"PORT=3000 curl localhost:3000", ex(3000)},

		// Several commands.
		{"cd web && npm run dev", ex(3001)},
		{"vite --port 3000 & next dev; rails s", ex(3000)},
		{"(vite) | tee log", def(5173)},
		{"vite --port 3000 || vite --port 3000", ex(3000)},
		{"vite --port 3000 && next dev -p 4000", ex(3000, 4000)},

		// Quoting.
		{`vite --port "3000"`, ex(3000)},
		{`PORT='3008' node 'my server.js'`, ex(3008)},
		{`echo "a; b" && vite`, def(5173)},
		{`vite --port\ 3000`, def(5173)},
	} {
		got := Ports(tc.cmd, lookup)
		if diff := cmp.Diff(tc.want, got); diff != "" {
			t.Errorf("Ports(%q) (-want +got):\n%s", tc.cmd, diff)
		}
	}
}

func TestPortsWithoutScripts(t *testing.T) {
	t.Parallel()
	if got := Ports("npm run dev", nil); len(got) != 0 {
		t.Errorf("npm run dev without scripts = %v, want none", got)
	}
	if diff := cmp.Diff(def(3000), Ports("npm start", nil)); diff != "" {
		t.Errorf("npm start without scripts (-want +got):\n%s", diff)
	}
}

func TestSplit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{`a "b c" 'd e' f\ g`, []string{"a", "b c", "d e", "f g"}},
		{`a"b"c`, []string{"abc"}},
		{`"it's" 'say "hi"' "a\"b"`, []string{"it's", `say "hi"`, `a"b`}},
		{"a && b || c; d | e & f", []string{"a", opToken, "b", opToken, "c", opToken, "d", opToken, "e", opToken, "f"}},
		{"a\n\tb", []string{"a", opToken, "b"}},
		{`'' x`, []string{"", "x"}},
		{"", nil},
	} {
		if diff := cmp.Diff(tc.want, split(tc.in)); diff != "" {
			t.Errorf("split(%q) (-want +got):\n%s", tc.in, diff)
		}
	}
}
