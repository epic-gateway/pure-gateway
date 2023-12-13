package trueingress

import (
	"fmt"
	"os/exec"

	"github.com/go-logr/logr"
)

const (
	ExplicitPacketForward int = 4
)

// runProgram is a little helper that runs a program and logs what's
// being run. If the result is non-zero then it logs the error, too.
func runProgram(l logr.Logger, program string, args ...string) error {
	cmd := exec.Command(program, args...)
	err := cmd.Run()
	if err != nil {
		l.Error(err, "runProgram", "command", cmd.String())
	} else {
		l.Info("runProgram", "command", cmd.String())
	}
	return err
}

// runScript is a little helper that runs a shell script and logs
// what's being run. If the result is non-zero then it logs the error,
// too.
func runScript(l logr.Logger, script string) error {
	return runProgram(l, "/bin/sh", "-c", script)
}

// cleanupQueueDiscipline removes our qdisc from the specified nic. It's useful
// for forcing SetBalancer to reload the filters which also
// initializes the maps.
func cleanupQueueDiscipline(l logr.Logger, nic string) error {
	// remove the clsact qdisc from the nic if it's there
	// tc qdisc del dev cni0 clsact
	return runScript(l, fmt.Sprintf("/usr/sbin/tc qdisc list dev %[1]s clsact | /usr/bin/grep clsact && /usr/sbin/tc qdisc del dev %[1]s clsact || true", nic))
}

// cleanupFilter removes our filters from the specified nic. It's useful
// for forcing SetBalancer to reload the filters which also
// initializes the maps.
func cleanupFilter(l logr.Logger, nic string, direction string) error {
	// add the pfc_{encap|decap}_tc filter to the nic if it's not already there
	// tc filter del dev cni0 egress
	return runScript(l, fmt.Sprintf("/usr/sbin/tc filter show dev %[1]s %[2]s | /usr/bin/grep 'pfc_encap_tc\\|pfc_decap_tc' && /usr/sbin/tc filter del dev %[1]s %[2]s || true", nic, direction))
}

// SetupNIC adds the PFC components to nic. direction should be either
// "ingress" or "egress". qid should be 0 or 1. flags is typically
// either 8 or 9 where 9 adds debug logging.
func SetupNIC(l logr.Logger, nic string, function string, direction string, qid int, flags int) error {

	// tc qdisc add dev nic clsact
	addQueueDiscipline(l, nic)

	// tc filter add dev nic ingress bpf direct-action object-file pfc_ingress_tc.o sec .text
	addFilter(l, nic, function, direction)

	// cli_cfg set nic 0 0 9
	configureTrueIngress(l, nic, qid, flags)

	return nil
}

func addQueueDiscipline(l logr.Logger, nic string) error {
	// add the clsact qdisc to the nic if it's not there
	return runScript(l, fmt.Sprintf("/usr/sbin/tc qdisc list dev %[1]s clsact | /usr/bin/grep clsact || /usr/sbin/tc qdisc add dev %[1]s clsact", nic))
}

func addFilter(l logr.Logger, nic string, function string, direction string) error {
	// add the pfc_{encap|decap}_tc filter to the nic if it's not already there
	return runScript(l, fmt.Sprintf("/usr/sbin/tc filter show dev %[1]s %[3]s | /usr/bin/grep pfc_%[2]s_tc || /usr/sbin/tc filter add dev %[1]s %[3]s bpf direct-action object-file /opt/acnodal/bin/pfc_%[2]s_tc.o sec .text", nic, function, direction))
}

func configureTrueIngress(l logr.Logger, nic string, qid int, flags int) error {
	return runScript(l, fmt.Sprintf("/opt/acnodal/bin/cli_cfg set %[1]s %[2]d %[3]d", nic, qid, flags))
}

// SetTunnel sets the parameters needed by one PFC tunnel.
func SetTunnel(l logr.Logger, tunnelID uint32, tunnelAddr string, myAddr string, tunnelPort int32) error {
	return runScript(l, fmt.Sprintf("/opt/acnodal/bin/cli_tunnel set %[1]d %[3]s %[4]d %[2]s %[4]d", tunnelID, tunnelAddr, myAddr, tunnelPort))
}
