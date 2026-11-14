//go:build windows

package hostapp

import "golang.org/x/sys/windows"

// openBrowser opens url in the default browser (ShellExecute: no console window appears).
func openBrowser(url string) error {
	verb, _ := windows.UTF16PtrFromString("open")
	target, err := windows.UTF16PtrFromString(url)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, target, nil, nil, windows.SW_SHOWNORMAL)
}
