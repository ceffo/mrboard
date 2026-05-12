package mrboardcmd

import (
	"fmt"
	"os"
	"time"
)

const defaultTimeout = 30 * time.Second

func loadTimeout() time.Duration {
	if v := os.Getenv("MRBOARD_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
		fmt.Fprintf(os.Stderr, "mrboard: invalid MRBOARD_TIMEOUT %q, using default\n", v)
	}
	return defaultTimeout
}
