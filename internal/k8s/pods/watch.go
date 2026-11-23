package pods

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/dash0hq/ditt0-cli-design-system/go/ditt0"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cli/browser"
	"github.com/dash0hq/dash0-api-client-go/profiles"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/color"
	"github.com/dash0hq/dash0-cli/internal/config"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/dash0hq/dash0-cli/internal/otlp"
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
	agg          string
	perPod       bool
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

func newWatchCmd(kind, plural string) *cobra.Command {
	flags := &watchFlags{agg: "total"}
	workload := kind != "pod"

	long := `Draw a pod's CPU and memory from Dash0 in the terminal, plus its requests, errors, duration and log counts when it has spans and logs.
The page refreshes every 30 seconds.
In the page, 1 to 4 pick 15m, 1h, 6h or 24h, r refreshes, o, t and l open the pod's metrics, traces and logs in Dash0, and q or Esc quits.

The pod is matched on the k8s.namespace.name and k8s.pod.name resource attributes.`
	example := `  # Watch a pod
  dash0 -X k8s pods watch <pod> -n <namespace>

  # Open on the last 6 hours, in Dash0's light colours
  dash0 -X k8s pods watch <pod> -n <namespace> --range 6h --light

  # Match the pod in one cluster only, with a named profile
  dash0 -X --profile <profile> k8s pods watch <pod> -n <namespace> --cluster <cluster>

  # Draw one frame to stdout and exit
  dash0 -X k8s pods watch <pod> -n <namespace> --once

  # Print the pod's series and Dash0 links as JSON
  dash0 -X k8s pods watch <pod> -n <namespace> -o json`
	if workload {
		long = `Draw the CPU and memory of a ` + kind + `'s pods from Dash0 in the terminal, plus their requests, errors, duration and log counts when they have spans and logs.
The page refreshes every 30 seconds.
In the page, 1 to 4 pick 15m, 1h, 6h or 24h, r refreshes, o, t and l open the pods' metrics, traces and logs in Dash0, and q or Esc quits.
a switches between the total of the pods and their average, minimum or maximum at each step, taken for usage, requests and limits apart, and p draws a line for each of the 8 busiest pods.
For CPU and memory, the busiest pods are those nearest their limits; for requests and duration, those with the highest mean.

The ` + kind + ` is matched on the k8s.namespace.name and k8s.` + kind + `.name resource attributes.`
		example = `  # Watch a ` + kind + `
  dash0 -X k8s ` + plural + ` watch <` + kind + `> -n <namespace>

  # Watch the maximum across its pods at each step
  dash0 -X k8s ` + plural + ` watch <` + kind + `> -n <namespace> --aggregate max

  # Watch it with a line per pod
  dash0 -X k8s ` + plural + ` watch <` + kind + `> -n <namespace> --per-pod

  # Print its series and Dash0 links as JSON
  dash0 -X k8s ` + plural + ` watch <` + kind + `> -n <namespace> -o json`
	}

	cmd := &cobra.Command{
		Use:   "watch <" + kind + "> --namespace <namespace>",
		Short: "[experimental] Watch a " + kind + "'s CPU, memory, requests and logs from Dash0",
		Long: long + `
Pass --cluster (or set DASH0_CLUSTER) to also match k8s.cluster.name.

The command also runs as a k9s plugin; see the k9s plugin section of the README.` + internal.CONFIG_HINT,
		Example: example,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			err := runWatch(cmd, kind, args[0], flags)
			if err != nil && flags.pauseOnError {
				// k9s clears the screen when a plugin exits: hold the error until it has been read
				fmt.Fprintln(os.Stderr, "Error: "+err.Error())
				waitForKey()
			}
			return err
		},
	}

	if workload {
		cmd.Flags().StringVar(&flags.agg, "aggregate", "total", "How the pods add up: "+strings.Join(aggs, ", "))
		cmd.Flags().BoolVar(&flags.perPod, "per-pod", false, "Open on a line per pod")
	}
	cmd.Flags().StringVarP(&flags.namespace, "namespace", "n", "", "The "+kind+"'s namespace (required)")
	cmd.Flags().StringVar(&flags.cluster, "cluster", os.Getenv(envCluster), "Dash0's k8s.cluster.name; empty matches the "+kind+" in any cluster (env: DASH0_CLUSTER)")
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

func runWatch(cmd *cobra.Command, kind, name string, flags *watchFlags) error {
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
	agg := strings.ToLower(flags.agg)
	if !slices.Contains(aggs, agg) {
		return fmt.Errorf("unknown aggregation %q (valid aggregations: %s)", flags.agg, strings.Join(aggs, ", "))
	}

	ctx := cmd.Context()
	// Fails up front, with the CLI's hints, on missing credentials or an OAuth profile that is not logged in
	if _, err := client.NewClientFromContext(ctx, flags.apiURL, flags.authToken); err != nil {
		return err
	}
	cmd.SilenceUsage = true

	p := &page{
		target:   target{cluster: flags.cluster, namespace: flags.namespace, kind: kind, name: name, context: flags.kubeContext, profile: profileName(ctx, flags), agg: agg},
		rangeIdx: rangeIdx,
		perPod:   flags.perPod,
		theme:    ditt0.Dark,
		apiURL:   client.ResolveApiUrl(ctx, flags.apiURL),
		dataset:  client.ResolveDataset(ctx, flags.dataset),
	}
	if flags.light {
		p.theme = ditt0.Light
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
	refresh := func(ctx context.Context, columns int) {
		c, err := connect()
		if err != nil {
			p.mu.Lock()
			p.loading, p.lastError = false, err.Error()
			p.failed = make([]string, len(panelsFor(p.target, p.perPod)))
			p.mu.Unlock()
			return
		}
		p.fetch(ctx, c, columns)
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
		refresh(ctx, columns)
		return p.printJSON(cmd.OutOrStdout())
	}

	if flags.once || !term.IsTerminal(int(os.Stdin.Fd())) {
		w, h := size()
		_, columns := p.render(w, h)
		refresh(ctx, columns)
		frame, _ := p.render(w, h)
		for _, l := range frame {
			if color.NoColor {
				fmt.Println(plain(l))
			} else {
				fmt.Println(l.ANSI())
			}
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
	pod, agg := "", p.target.agg
	if p.target.isPod() {
		pod, agg = p.target.name, ""
	}
	out := struct {
		Namespace  string                `json:"namespace"`
		Kind       string                `json:"kind"`
		Name       string                `json:"name"`
		Pod        string                `json:"pod,omitempty"`
		Aggregate  string                `json:"aggregate,omitempty"`
		Cluster    string                `json:"cluster,omitempty"`
		Step       string                `json:"step"`
		Timestamps []string              `json:"timestamps"`
		Series     map[string][]*float64 `json:"series"`
		Failed     []string              `json:"failed,omitempty"`
		Links      map[string]string     `json:"links"`
	}{p.target.namespace, p.target.kind, p.target.name, pod, agg, p.target.cluster, p.win.step.String(), timestamps, p.data, p.failed, links}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// The interactive page, on the alternate screen so the terminal (or k9s) comes back as it was
func (p *page) run(ctx context.Context, size func() (int, int), refresh func(ctx context.Context, columns int)) error {
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
		if !p.target.isPod() {
			hints = "a total/avg/min/max · p per pod · " + hints
		}
		if overflow {
			hints = fmt.Sprintf("↑↓ scroll (%d%%) · ", 100*(p.scroll+body)/len(frame)) + hints
		}
		var sb strings.Builder
		sb.WriteString("\x1b[H")
		for i := 0; i < body; i++ {
			var l ditt0.Line
			if p.scroll+i < len(frame) {
				l = frame[p.scroll+i]
			}
			sb.WriteString(l.Cut(w).ANSI() + "\x1b[K\r\n")
		}
		sb.WriteString(ditt0.Line{{Text: " " + hints, Style: ditt0.DimStyle}}.Cut(w).ANSI() + "\x1b[K")
		fmt.Print(sb.String())
		return columns
	}
	// A new refresh cancels the one still running, so holding r sends one refresh's requests, not dozens
	cancelRefresh := func() {}
	defer func() { cancelRefresh() }()
	ask := func() {
		columns := draw()
		cancelRefresh()
		var refreshCtx context.Context
		refreshCtx, cancelRefresh = context.WithCancel(ctx)
		go func() { refresh(refreshCtx, columns); poke() }()
	}

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
				k := nextKey(in)
				keys <- k
				in = in[len(k):]
			}
		}
	}()
	// SIGWINCH does not exist on Windows; otlp.InstallWidthRefresh is a no-op there
	winch := make(chan struct{}, 1)
	winchCtx, stopWinch := context.WithCancel(ctx)
	defer stopWinch()
	otlp.InstallWidthRefresh(winchCtx, winch)
	ticker := time.NewTicker(refreshEvery)
	defer ticker.Stop()
	// A range or view change starts empty, and a fetch for the old one that lands before the next one starts is
	// dropped. Dash0 combines the pods, so a switch asks it again
	restart := func(change func()) {
		p.mu.Lock()
		change()
		p.data, p.at, p.failed = nil, time.Time{}, nil
		p.gen++
		p.mu.Unlock()
		ticker.Reset(refreshEvery)
		ask()
	}

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
			case "a", "p":
				if p.target.isPod() {
					break
				}
				restart(func() {
					if k == "a" {
						p.target.agg = aggs[(slices.Index(aggs, p.target.agg)+1)%len(aggs)]
					} else {
						p.perPod = !p.perPod
					}
				})
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
				restart(func() { p.rangeIdx = int(k[0] - '1') })
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

// The first key in what the terminal sent: one byte, or a whole escape sequence (CSI up to its final byte in
// 0x40-0x7E, SS3 and its one byte, Alt and its key), so an F key or Shift-Tab is ignored instead of read as Esc
func nextKey(in string) string {
	if in[0] != '\x1b' || len(in) == 1 {
		return in[:1]
	}
	switch in[1] {
	case '[':
		for i := 2; i < len(in); i++ {
			if in[i] >= 0x40 && in[i] <= 0x7e {
				return in[:i+1]
			}
		}
		return in
	case 'O':
		return in[:min(3, len(in))]
	default:
		return in[:2]
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
