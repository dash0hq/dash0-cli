//go:build windows

package otlp

import "strconv"

// portInUseHint returns the platform-appropriate command the user can
// run to identify the process holding a given TCP port. `netstat -ano`
// is universally available on Windows; piping through `findstr` filters
// to the line(s) for the port, and the last column is the owning PID,
// which `tasklist /FI "PID eq <PID>"` can resolve to a process name.
func portInUseHint(port int) string {
	return "netstat -ano | findstr :" + strconv.Itoa(port)
}

// lookupPortHolder on Windows is a no-op stub. Identifying the holder
// of a TCP port on Windows requires `netstat -ano` + `tasklist`, both
// shellouts with parsing overhead and brittle column layouts that vary
// across locales. The proxy's audience is local development primarily
// on macOS/Linux; Windows users get the platform-appropriate hint
// (printed via portInUseHint above) and run the lookup themselves.
// Filling this in is a follow-up if Windows usage warrants it.
func lookupPortHolder(_ int) (string, int, bool) {
	return "", 0, false
}
