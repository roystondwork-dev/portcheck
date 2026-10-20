package killer_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/roystondwork-dev/portcheck/pkg/killer"
)

func ExampleKiller_Kill() {
	var pid int32 = 4242 // e.g. from inspect

	res, err := killer.New().Kill(context.Background(), pid, killer.Options{Grace: 5 * time.Second})
	switch {
	case errors.Is(err, killer.ErrProtected):
		fmt.Println("refused:", err)
	case errors.Is(err, killer.ErrPermission):
		fmt.Println("needs root/Administrator")
	case err != nil:
		fmt.Println("failed:", err)
	default:
		fmt.Println(res.Name, res.Outcome)
	}
}

func ExampleIsProtectedName() {
	fmt.Println(killer.IsProtectedName("launchd"), killer.IsProtectedName("node"))
	// Output: true false
}
