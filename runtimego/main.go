package main

/*
#include <stdint.h>
*/
import "C"

import (
	"fmt"
)

//export tsnative_console_log_f64
func tsnative_console_log_f64(value C.double) {
	fmt.Printf("%.17g\n", float64(value))
}

func main() {}
