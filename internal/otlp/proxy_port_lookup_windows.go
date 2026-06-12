//go:build windows

package otlp

// lookupPortHolder on Windows is a no-op stub. Identifying the holder
// of a TCP port on Windows requires `netstat -ano` + `tasklist`, both
// shellouts with parsing overhead and brittle column layouts that vary
// across locales. The proxy's audience is local development primarily
// on macOS/Linux; Windows users get the generic error message that
// already names the port and the override flag. Filling this in is a
// follow-up if Windows usage warrants it.
func lookupPortHolder(_ int) (string, int, bool) {
	return "", 0, false
}
