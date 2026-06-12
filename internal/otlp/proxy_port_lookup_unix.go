//go:build !windows

package otlp

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// portLookupTimeout bounds the lsof invocation. lsof is fast in practice
// (single-port query is < 100ms on a loaded laptop), so a half-second
// ceiling is generous and keeps the proxy responsive when lsof is
// missing or hangs.
const portLookupTimeout = 500 * time.Millisecond

// portInUseHint returns the platform-appropriate command the user can
// run to identify the process holding a given TCP port. Surfaced in the
// fallback path of portInUseError when lookupPortHolder couldn't
// auto-resolve the holder.
func portInUseHint(port int) string {
	return "lsof -iTCP:" + strconv.Itoa(port) + " -sTCP:LISTEN"
}

// lookupPortHolder identifies the process holding a listening TCP port
// on 127.0.0.1 via lsof. Returns the command name and PID when
// identification succeeds. ok=false means lsof was unavailable, didn't
// return a clean parse, or didn't find a holder — callers fall back to
// a generic error message.
//
// The command is `lsof -iTCP:<port> -sTCP:LISTEN -P -n -F pcn`. The
// `-F pcn` field-output mode prints one attribute per line with a
// single-letter prefix: `p` = PID, `c` = command, `n` = NAME (host:port).
// We pick the first PID/command pair and ignore the rest, which is
// adequate for the bind-collision case where only one process owns a
// listening port at a time.
func lookupPortHolder(port int) (name string, pid int, ok bool) {
	ctx, cancel := context.WithTimeout(context.Background(), portLookupTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "lsof",
		"-iTCP:"+strconv.Itoa(port),
		"-sTCP:LISTEN",
		"-P", "-n",
		"-F", "pcn",
	)
	out, err := cmd.Output()
	if err != nil {
		return "", 0, false
	}

	for _, line := range strings.Split(string(out), "\n") {
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'p':
			parsed, parseErr := strconv.Atoi(line[1:])
			if parseErr == nil {
				pid = parsed
			}
		case 'c':
			name = line[1:]
		}
		if pid != 0 && name != "" {
			return name, pid, true
		}
	}
	return "", 0, false
}
