# Go library

The portcheck CLI is a thin layer over public packages you can import:

```sh
go get github.com/roystondwork-dev/portcheck@latest
```

| Package | Purpose |
|---|---|
| [`pkg/portspec`](https://pkg.go.dev/github.com/roystondwork-dev/portcheck/pkg/portspec) | parse and iterate port specs like `22,80,8000-8100,common` |
| [`pkg/inspect`](https://pkg.go.dev/github.com/roystondwork-dev/portcheck/pkg/inspect) | list sockets with their owning processes |
| [`pkg/freeport`](https://pkg.go.dev/github.com/roystondwork-dev/portcheck/pkg/freeport) | check and find free ports |
| [`pkg/killer`](https://pkg.go.dev/github.com/roystondwork-dev/portcheck/pkg/killer) | stop processes gracefully, with safety guards |
| [`pkg/scan`](https://pkg.go.dev/github.com/roystondwork-dev/portcheck/pkg/scan) | rate-limited TCP connect scanning |
| [`pkg/waitfor`](https://pkg.go.dev/github.com/roystondwork-dev/portcheck/pkg/waitfor) | poll until a port opens (or any condition holds) |

The packages have no dependency on the CLI or TUI libraries (enforced by a linter rule),
and use only the standard library plus gopsutil and `golang.org/x/{sync,sys,time}`.
The examples below are runnable `Example` tests in the repository, so they are
checked by CI.

!!! note "Stability"
    Before v1.0.0, minor releases may change these APIs; changes are called out in the
    [changelog](https://github.com/roystondwork-dev/portcheck/blob/main/CHANGELOG.md).

## Port specs

```go
spec, err := portspec.Parse("22,80,8000-8002")
if err != nil {
	log.Fatal(err)
}
fmt.Println(spec.Len(), spec.Contains(8001)) // 5 true
for port := range spec.All() {
	fmt.Print(port, " ") // 22 80 8000 8001 8002
}
```

## Who is listening on a port?

```go
res, err := inspect.NewSystem().List(ctx, inspect.Filter{
	Ports:         portspec.Single(3000),
	ListeningOnly: true,
})
if err != nil {
	log.Fatal(err)
}
for _, s := range res.Sockets {
	if s.Process != nil {
		fmt.Printf("%s %s pid=%d %s\n", s.Proto, s.Local, s.PID, s.Process.Name)
	}
}
if res.Partial {
	fmt.Println(inspect.PrivilegeHint())
}
```

To test code that uses `inspect`, pass an `inspect.FakeProvider` to `inspect.New`
instead of using `NewSystem`.

## Free ports

```go
f := freeport.New()
st, err := f.Check(ctx, 5432, inspect.TCP)
if err != nil {
	log.Fatal(err)
}
fmt.Println(st.Availability) // free, in-use or denied

ports, err := f.Find(ctx, portspec.MustParse("3000-3999"), inspect.TCP, 3)
```

## Stopping a process

```go
res, err := killer.New().Kill(ctx, pid, killer.Options{Grace: 5 * time.Second})
switch {
case errors.Is(err, killer.ErrProtected):
	// PID 0/1, ourselves, our parent, or a system process
case errors.Is(err, killer.ErrPermission):
	// needs root / Administrator
case err == nil:
	fmt.Println(res.Outcome) // terminated, killed or already-exited
}
```

## Scanning

```go
targets, err := scan.Expand(ctx, net.DefaultResolver, []string{"db.internal", "10.0.0.0/30"}, scan.DefaultMaxHosts)
if err != nil {
	log.Fatal(err)
}
for r := range scan.Scan(ctx, targets, portspec.MustParse("5432,6379"), scan.Options{}) {
	fmt.Println(r.Target, r.Port, r.State)
}
```

## Waiting for a service

```go
ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
defer cancel()
r := waitfor.Poll(ctx, 250*time.Millisecond,
	waitfor.DialCheck(&net.Dialer{}, "tcp", "localhost:5432", time.Second))
fmt.Println(r.Ready, r.Attempts, r.Waited)
```

`scan.Options{}` uses safe defaults: 100 concurrent connections, a 1s timeout and at
most 500 connections per second. Results stream in completion order; the channel
closes when the scan finishes or `ctx` is cancelled.
