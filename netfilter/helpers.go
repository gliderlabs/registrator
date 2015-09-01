package netfilter

import (
	"os/exec"
	"strings"
	"syscall"
)

const (
	ip6tablesPath = "/usr/sbin/ip6tables"
	ipsetPath     = "/usr/sbin/ipset"
)

func checkTestError(err error) (bool, error) {
	switch {
	case err == nil:
		return true, nil
	case err.(*exec.ExitError).Sys().(syscall.WaitStatus).ExitStatus() == 1:
		return false, nil
	default:
		return false, err
	}
}

func iptablesRun(ipcmd string) error {
	args := strings.Fields(ipcmd)
	if firewalldRunning && !strings.HasPrefix(ipcmd, "-t") {
		Passthrough(args)
	} else {
		cmd := exec.Cmd{Path: ip6tablesPath, Args: append([]string{ip6tablesPath}, args...)}
		if err := cmd.Run(); err != nil {
			return err.(*exec.ExitError)
		}
	}
	return nil
}

func ipsetRun(ipcmd string) error {
	args := strings.Fields(ipcmd)
	cmd := exec.Cmd{Path: ipsetPath, Args: append([]string{ipsetPath}, args...)}
	if err := cmd.Run(); err != nil {
		return err.(*exec.ExitError)
	}
	return nil
}

func ipsetHost(command string, set string, ip string, proto string, port string) error {
	cmd := "-! " + command + " " + set + " " + ip + "," + proto + ":" + port
	err := ipsetRun(cmd)
	if err != nil {
		return err
	}
	cmd = "-! " + command + " " + set + " " + ip + "," + "icmpv6:128/0"
	err = ipsetRun(cmd)
	if err != nil {
		return err
	}
	return nil
}

func iptablesInit(chain string, set string) error {
	exists, err := checkTestError(iptablesRun("-t filter -C " + chain + " -o docker0 -m set --match-set " + set + " dst,dst -j ACCEPT --wait"))
	if err != nil {
		return err
	}
	if !exists {
		iptablesRun("-A " + chain + " -o docker0 -m set --match-set " + set + " dst,dst -j ACCEPT --wait")
	}
	exists, err = checkTestError(iptablesRun("-t filter -C " + chain + " -o docker0 -j DROP --wait"))
	if err != nil {
		return err
	}
	if !exists {
		iptablesRun("-A " + chain + " -o docker0 -j DROP --wait")
	}

	return nil
}

func ipsetInit(set string) error {
	err := ipsetRun("-! create " + set + " hash:ip,port family inet6")
	if err != nil {
		return err
	}
	err = ipsetRun("-! flush " + set)
	if err != nil {
		return err
	}
	return nil
}
