package pods

import (
	"github.com/spf13/cobra"
)

// What the page can watch: kind as Dash0's resource attribute k8s.<kind>.name has it, plural as k9s passes it in
// $RESOURCE_NAME, and kubectl's singular and short names
var workloads = []struct {
	kind, plural string
	aliases      []string
}{
	{"pod", "pods", []string{"pod", "po"}},
	{"deployment", "deployments", []string{"deployment", "deploy"}},
	{"statefulset", "statefulsets", []string{"statefulset", "sts"}},
	{"daemonset", "daemonsets", []string{"daemonset", "ds"}},
	{"replicaset", "replicasets", []string{"replicaset", "rs"}},
	{"job", "jobs", []string{"job"}},
	{"cronjob", "cronjobs", []string{"cronjob", "cj"}},
}

// NewCmds creates a command group per kind: k8s pods, k8s deployments, k8s statefulsets, ...
func NewCmds() []*cobra.Command {
	var cmds []*cobra.Command
	for _, w := range workloads {
		cmd := &cobra.Command{
			Use:     w.plural,
			Aliases: w.aliases,
			Short:   "[experimental] Watch Kubernetes " + w.plural + " in Dash0",
			Long:    `Watch the telemetry Dash0 holds for a Kubernetes ` + w.kind + `.`,
		}
		cmd.AddCommand(newWatchCmd(w.kind))
		cmds = append(cmds, cmd)
	}
	return cmds
}
