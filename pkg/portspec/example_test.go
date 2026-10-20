package portspec_test

import (
	"fmt"

	"github.com/roystondwork-dev/portcheck/pkg/portspec"
)

func ExampleParse() {
	spec, err := portspec.Parse("22,80,8000-8002")
	if err != nil {
		panic(err)
	}
	fmt.Println(spec.Len(), spec.Contains(8001))
	for port := range spec.All() {
		fmt.Print(port, " ")
	}
	fmt.Println()
	// Output:
	// 5 true
	// 22 80 8000 8001 8002
}

func ExampleSpec_String() {
	// Overlapping and adjacent entries are merged into a canonical form.
	fmt.Println(portspec.MustParse("443, 80-82, 81, 83"))
	// Output: 80-83,443
}

func ExamplePresets() {
	fmt.Println(portspec.Presets())
	fmt.Println(portspec.MustParse("well-known").Len())
	// Output:
	// [all common dynamic registered well-known]
	// 1023
}
