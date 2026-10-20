package scan_test

import (
	"context"
	"fmt"
	"net"

	"github.com/roystondwork-dev/portcheck/pkg/portspec"
	"github.com/roystondwork-dev/portcheck/pkg/scan"
)

func ExampleExpand() {
	// IP and CIDR targets need no resolver lookups; duplicates are dropped.
	targets, err := scan.Expand(context.Background(), net.DefaultResolver, []string{"10.0.0.0/30", "10.0.0.1"}, scan.DefaultMaxHosts)
	if err != nil {
		panic(err)
	}
	for _, t := range targets {
		fmt.Println(t)
	}
	// Output:
	// 10.0.0.0
	// 10.0.0.1
	// 10.0.0.2
	// 10.0.0.3
}

// Only scan hosts you own or are authorized to test.
func ExampleScan() {
	ctx := context.Background()
	targets, err := scan.Expand(ctx, net.DefaultResolver, []string{"localhost"}, 1)
	if err != nil {
		panic(err)
	}
	var sum scan.Summary
	for r := range scan.Scan(ctx, targets, portspec.MustParse("5432,6379"), scan.Options{}) {
		sum.Add(r)
		fmt.Println(r.Target, r.Port, r.State, r.Service)
	}
	fmt.Println(sum.Open, "open")
}
