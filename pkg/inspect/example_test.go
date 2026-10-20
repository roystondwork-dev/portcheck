package inspect_test

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/portspec"
)

// Programs use inspect.NewSystem(); this example uses a FakeProvider so its
// output is deterministic. FakeProvider is also handy in your own tests.
func ExampleInspector_List() {
	provider := &inspect.FakeProvider{
		SocketList: []inspect.RawSocket{
			{Proto: inspect.TCP, Local: netip.MustParseAddrPort("127.0.0.1:3000"), State: inspect.StateListen, PID: 4242},
			{Proto: inspect.TCP, Local: netip.MustParseAddrPort("0.0.0.0:22"), State: inspect.StateListen}, // owner hidden
		},
		Processes: map[int32]*inspect.ProcessInfo{4242: {PID: 4242, Name: "node"}},
	}

	res, err := inspect.New(provider).List(context.Background(), inspect.Filter{
		Ports:         portspec.MustParse("1-5000"),
		ListeningOnly: true,
	})
	if err != nil {
		panic(err)
	}
	for _, s := range res.Sockets {
		name := "?"
		if s.Process != nil {
			name = s.Process.Name
		}
		fmt.Println(s.Proto, s.Local, s.PID, name)
	}
	fmt.Println("partial:", res.Partial)
	// Output:
	// tcp 0.0.0.0:22 0 ?
	// tcp 127.0.0.1:3000 4242 node
	// partial: true
}
