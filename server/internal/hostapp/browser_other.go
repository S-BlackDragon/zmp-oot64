//go:build !windows

package hostapp

import "errors"

// openBrowser: on other systems the server only prints the address (it runs from a terminal or a service).
func openBrowser(string) error { return errors.New("not on Windows") }
