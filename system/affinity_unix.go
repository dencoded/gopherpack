// +build aix dragonfly freebsd linux netbsd openbsd solaris

package system

import "golang.org/x/sys/unix"

func SetAffinity(cpuCore int) error {
	cpu := &unix.CPUSet{}
	cpu.Set(cpuCore)
	return unix.SchedSetaffinity(0, cpu)
}
