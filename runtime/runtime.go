package runtime

import (
	"fmt"
)

func consoleLogF64(value float64) {
	fmt.Printf("%.17g\n", value)
}
