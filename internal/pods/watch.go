package pods

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cli/browser"
	"github.com/dash0hq/dash0-api-client-go/profiles"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/config"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const envCluster = "DASH0_CLUSTER"

type watchFlags struct {
	apiURL       string
	authToken    string
	dataset      string
	namespace    string
	kubeContext  string
	cluster      string
	rangeLabel   string
	output       string
	light        bool
	once         bool
	pauseOnError bool
}

type watchFormat string

const (
	watchFormatPage watchFormat = "page"
	watchFormatJSON watchFormat = "json"
)

func parseWatchFormat(s string) (watchFormat, error) {
	switch strings.ToLower(s) {
	case "":
		if agentmode.Enabled {
			return watchFormatJSON, nil
		}
		return watchFormatPage, nil
	case "page":
		return watchFormatPage, nil
	case "json":
		return watchFormatJSON, nil
	default:
		return "", fmt.Errorf("unknown output format: %s (valid formats: page, json)", s)
	}
}

func newWatchCmd() *cobra.Command {
	flags := &watchFlags{}

	cmd := &cobra.Command{
		Use:   "watch <pod> --namespace <namespace>",
		Short: "[experimental] Watch a pod's CPU, memory, requests and logs from Dash0",
		Long: `Draw a pod's CPU and memory from Dash0 in the terminal, plus its requests, errors, duration and log counts when it has spans and logs.
The page refreshes every 30 seconds.
In the page, 1 to 4 pick 15m, 1h, 6h or 24h, r refreshes, o, t and l open the pod's metrics, traces and logs in Dash0, and q or Esc quits.

The pod is matched on the k8s.namespace.name and k8s.pod.name resource attributes.
Pass --cluster (or set DASH0_CLUSTER) to also match k8s.cluster.name.

The command also runs as a k9s plugin; see the k9s plugin section of the README.` + internal.CONFIG_HINT,
		Example: `  # Watch a pod
  dash0 -X pods watch <pod> -n <namespace>

  # Open on the last 6 hours, in Dash0's light colours
  dash0 -X pods watch <pod> -n <namespace> --range 6h --light

  # Match the pod in one cluster only, with a named profile
  dash0 -X --profile <profile> pods watch <pod> -n <namespace> --cluster <cluster>

  # Draw one frame to stdout and exit
  dash0 -X pods watch <pod> -n <namespace> --once

  # Print the pod's series and Dash0 links as JSON
  dash0 -X pods watch <pod> -n <namespace> -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			err := runWatch(cmd, args[0], flags)
			if err != nil && flags.pauseOnError {
				// k9s clears the screen when a plugin exits: hold the error until it has been read
				fmt.Fprintln(os.Stderr, "Error: "+err.Error())
				waitForKey()
			}
			return err
		},
	}

	cmd.Flags().StringVarP(&flags.namespace, "namespace", "n", "", "The pod's namespace (required)")
	cmd.Flags().StringVar(&flags.cluster, "cluster", os.Getenv(envCluster), "Dash0's k8s.cluster.name; empty matches the pod in any cluster (env: DASH0_CLUSTER)")
	cmd.Flags().StringVar(&flags.kubeContext, "context", "", "The kube context, shown in the header")
	cmd.Flags().StringVar(&flags.rangeLabel, "range", "1h", "The time range the page opens on: 15m, 1h, 6h or 24h")
	cmd.Flags().BoolVar(&flags.light, "light", os.Getenv("DITT0_THEME") == "light", "Draw in Dash0's light colours (env: DITT0_THEME=light)")
	cmd.Flags().BoolVar(&flags.once, "once", false, "Draw one frame to stdout and exit, at COLUMNS x LINES when stdout is not a terminal")
	cmd.Flags().BoolVar(&flags.pauseOnError, "pause-on-error", false, "Wait for a key before exiting on an error, so k9s does not clear it")
	cmd.Flags().StringVarP(&flags.output, "output", "o", "", "Output format: page, json (default: page; json in agent mode)")
	cmd.Flags().StringVar(&flags.apiURL, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.authToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVar(&flags.dataset, "dataset", "", "Dataset to query")
	_ = cmd.MarkFlagRequired("namespace")

	return cmd
}

func runWatch(cmd *cobra.Command, pod string, flags *watchFlags) error {
	if err := experimental.RequireExperimental(cmd); err != nil {
		return err
	}
	format, err := parseWatchFormat(flags.output)
	if err != nil {
		return err
	}
	rangeIdx := -1
	for i, r := range ranges {
		if r.label == flags.rangeLabel {
			rangeIdx = i
		}
	}
	if rangeIdx < 0 {
		return fmt.Errorf("unknown range %q (valid ranges: 15m, 1h, 6h, 24h)", flags.rangeLabel)
	}

	ctx := cmd.Context()
	// Fails up front, with the CLI's hints, on missing credentials or an OAuth profile that is not logged in
	if _, err := client.NewClientFromContext(ctx, flags.apiURL, flags.authToken); err != nil {
		return err
	}
	cmd.SilenceUsage = true

	p := &page{
		target:   target{cluster: flags.cluster, namespace: flags.namespace, pod: pod, context: flags.kubeContext, profile: profileName(ctx, flags)},
		rangeIdx: rangeIdx,
		theme:    dark,
		apiURL:   client.ResolveApiUrl(ctx, flags.apiURL),
		dataset:  client.ResolveDataset(ctx, flags.dataset),
	}
	if flags.light {
		p.theme = light
	}

	// Resolved before every refresh, so an OAuth access token is renewed while the page stays open
	connect := func() (promClient, error) {
		cfg, err := client.NewRawHTTPConfig(ctx, flags.apiURL, flags.authToken)
		if err != nil {
			return promClient{}, err
		}
		return promClient{
			base:      prometheusURL(cfg.ApiUrl),
			token:     cfg.AuthToken,
			userAgent: cfg.UserAgent,
			dataset:   p.dataset,
			http:      &http.Client{Timeout: 25 * time.Second},
		}, nil
	}
	refresh := func(columns int) {
		c, err := connect()
		if err != nil {
			p.mu.Lock()
			p.loading, p.lastError = false, err.Error()
			p.failed = make([]string, len(panels))
			p.mu.Unlock()
			return
		}
		p.fetch(c, columns)
	}

	fd := int(os.Stdout.Fd())
	size := func() (int, int) {
		w, h, err := term.GetSize(fd)
		if err != nil || w <= 0 {
			// Not a terminal, as with --once into a file: COLUMNS and LINES say the size to draw at
			w, h = 120, 50
			if n, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && n > 0 {
				w = n
			}
			if n, err := strconv.Atoi(os.Getenv("LINES")); err == nil && n > 0 {
				h = n
			}
		}
		return w, h
	}

	if format == watchFormatJSON {
		_, columns := p.render(size())
		refresh(columns)
		return p.printJSON(cmd.OutOrStdout())
	}

	if flags.once || !term.IsTerminal(int(os.Stdin.Fd())) {
		w, h := size()
		_, columns := p.render(w, h)
		refresh(columns)
		frame, _ := p.render(w, h)
		for _, l := range frame {
			fmt.Println(l.ansi())
		}
		return p.allFailed()
	}

	return p.run(ctx, size, refresh)
}

// The profile the header names: none when the environment or the flags supply the token
func profileName(ctx context.Context, flags *watchFlags) string {
	if sel := config.ProfileSelectorFromContext(ctx); sel.IsSet() {
		return sel.Name
	}
	if flags.authToken != "" || os.Getenv(profiles.EnvAuthToken) != "" {
		return ""
	}
	store, err := profiles.NewStore()
	if err != nil {
		return ""
	}
	active, err := store.GetActiveProfile()
	if err != nil {
		return ""
	}
	return active.Name
}

// An error when no panel could be loaded, so a scripted --once or -o json exits non-zero
func (p *page) allFailed() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.at.IsZero() && len(p.failed) > 0 {
		return fmt.Errorf("failed to query Dash0: %s", p.lastError)
	}
	return nil
}

func (p *page) printJSON(w io.Writer) error {
	if err := p.allFailed(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	links := map[string]string{}
	for _, l := range p.links() {
		links[strings.ToLower(strings.TrimPrefix(l.label, "Open "))] = l.url
	}
	times := p.win.times()
	timestamps := make([]string, len(times))
	for i, t := range times {
		timestamps[i] = t.UTC().Format(time.RFC3339)
	}
	out := struct {
		Namespace  string                `json:"namespace"`
		Pod        string                `json:"pod"`
		Cluster    string                `json:"cluster,omitempty"`
		Step       string                `json:"step"`
		Timestamps []string              `json:"timestamps"`
		Series     map[string][]*float64 `json:"series"`
		Failed     []string              `json:"failed,omitempty"`
		Links      map[string]string     `json:"links"`
	}{p.target.namespace, p.target.pod, p.target.cluster, p.win.step.String(), timestamps, p.data, p.failed, links}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// The interactive page, on the alternate screen so the terminal (or k9s) comes back as it was
func (p *page) run(ctx context.Context, size func() (int, int), refresh func(columns int)) error {
	state, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	fmt.Print("\x1b[?1049h\x1b[?25l")
	defer func() {
		fmt.Print("\x1b[?25h\x1b[?1049l")
		_ = term.Restore(int(os.Stdin.Fd()), state)
	}()
	// The browser's own output would draw over the page
	browser.Stdout, browser.Stderr = io.Discard, io.Discard

	redraw := make(chan struct{}, 1)
	poke := func() {
		select {
		case redraw <- struct{}{}:
		default:
		}
	}
	// The page scrolls under a fixed last row of key hints
	draw := func() int {
		w, h := size()
		frame, columns := p.render(w, h)
		body := max(1, h-1)
		overflow := len(frame) > body
		p.scroll = max(0, min(p.scroll, len(frame)-body))
		hints := "o metrics · t traces · l logs in Dash0 · 1-4 range · r refresh · q quit"
		if overflow {
			hints = fmt.Sprintf("↑↓ scroll (%d%%) · ", 100*(p.scroll+body)/len(frame)) + hints
		}
		var sb strings.Builder
		sb.WriteString("\x1b[H")
		for i := 0; i < body; i++ {
			var l line
			if p.scroll+i < len(frame) {
				l = frame[p.scroll+i]
			}
			sb.WriteString(l.cut(w).ansi() + "\x1b[K\r\n")
		}
		sb.WriteString(line{{" " + hints, dimStyle}}.cut(w).ansi() + "\x1b[K")
		fmt.Print(sb.String())
		return columns
	}
	ask := func() {
		columns := draw()
		go func() { refresh(columns); poke() }()
	}

	// Keys as the terminal sends them: one byte, or an escape sequence for an arrow or page key
	keys := make(chan string)
	go func() {
		buf := make([]byte, 64)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				close(keys)
				return
			}
			for in := string(buf[:n]); in != ""; {
				k := in[:1]
				if strings.HasPrefix(in, "\x1b[") || strings.HasPrefix(in, "\x1bO") {
					end := strings.IndexAny(in[2:], "ABCDHF~")
					if end >= 0 {
						k = in[:end+3]
					}
				}
				keys <- k
				in = in[len(k):]
			}
		}
	}()
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)
	ticker := time.NewTicker(refreshEvery)
	defer ticker.Stop()

	ask()
	for {
		select {
		case <-ctx.Done():
			return nil
		case k, ok := <-keys:
			if !ok {
				return nil
			}
			_, h := size()
			screen := max(1, h-2)
			switch k {
			case "q", "Q", "\x03", "\x1b":
				return nil
			case "r":
				ask()
			case "o", "t", "l":
				p.mu.Lock()
				links := p.links()
				p.mu.Unlock()
				for _, l := range links {
					if l.key == k {
						_ = browser.OpenURL(l.url)
					}
				}
			case "\x1b[A", "\x1bOA", "k":
				p.scroll--
				draw()
			case "\x1b[B", "\x1bOB", "j":
				p.scroll++
				draw()
			case "\x1b[5~", "b":
				p.scroll -= screen
				draw()
			case "\x1b[6~", " ":
				p.scroll += screen
				draw()
			case "\x1b[H", "\x1bOH", "g":
				p.scroll = 0
				draw()
			case "\x1b[F", "\x1bOF", "G":
				p.scroll = 1 << 20
				draw()
			case "1", "2", "3", "4":
				p.mu.Lock()
				p.rangeIdx = int(k[0] - '1')
				// A range change starts empty
				p.data, p.at, p.failed = nil, time.Time{}, nil
				p.mu.Unlock()
				ticker.Reset(refreshEvery)
				ask()
			}
		case <-ticker.C:
			ask()
		case <-winch:
			draw()
		case <-redraw:
			draw()
		}
	}
}

// The Prometheus API under a Dash0 API URL. A URL that already names it is kept, as the k9s plugin's
// DASH0_API_URL was written before it read profiles
func prometheusURL(apiURL string) string {
	base := strings.TrimRight(apiURL, "/")
	if strings.HasSuffix(base, "/api/prometheus") {
		return base
	}
	return base + "/api/prometheus"
}

func waitForKey() {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return
	}
	fmt.Fprint(os.Stderr, "\nPress any key to go back to k9s")
	if s, err := term.MakeRaw(int(os.Stdin.Fd())); err == nil {
		_, _ = os.Stdin.Read(make([]byte, 1))
		_ = term.Restore(int(os.Stdin.Fd()), s)
	}
}
