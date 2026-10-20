package freeport_test

import (
	"context"
	"fmt"

	"github.com/roystondwork-dev/portcheck/pkg/freeport"
	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/portspec"
)

// Results depend on the machine, so this example has no checked output.
func ExampleFinder() {
	ctx := context.Background()
	f := freeport.New()

	st, err := f.Check(ctx, 5432, inspect.TCP)
	if err != nil {
		panic(err)
	}
	fmt.Println("5432 is", st.Availability)
	for _, h := range st.Holders {
		fmt.Println("held by pid", h.PID)
	}

	ports, err := f.Find(ctx, portspec.MustParse("3000-3999"), inspect.TCP, 3)
	if err != nil {
		panic(err)
	}
	fmt.Println("free:", ports)
}
