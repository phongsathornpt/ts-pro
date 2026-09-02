package runtimego

import (
	"fmt"
)

func tsnative_console_log_f64(value float64) {
	fmt.Printf("%.17g\n", value)
}
