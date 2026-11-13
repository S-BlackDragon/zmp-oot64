//go:build !windows

package server

import (
	"os"
	"strconv"
	"strings"
)

// processMemoryKB returns what the operating system counts for this process: the memory it has in RAM right now
// (what docker stats shows) and its private data. 0, 0 when it cannot be read.
func processMemoryKB() (resident, private uint64) {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		v, _ := strconv.ParseUint(f[1], 10, 64)
		switch f[0] {
		case "VmRSS:":
			resident = v
		case "RssAnon:":
			private = v
		}
	}
	return resident, private
}
