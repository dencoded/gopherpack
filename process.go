package gopherpack

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func forkProcess(envValues []string) (*os.Process, error) {
	// get file path to current binary
	filePath, err := exec.LookPath(os.Args[0])
	if err != nil {
		return nil, err
	}

	// current dir
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	// inherit stdin, stdout and stderr by child process
	files := make([]*os.File, 3)
	files[syscall.Stdin] = os.Stdin
	files[syscall.Stdout] = os.Stdout
	files[syscall.Stderr] = os.Stderr

	// prepare environment for child process
	env := []string{}
	// copy current environment vars but remove all existing gopherpack vars if any
	for _, curEnvVar := range os.Environ() {
		if strings.HasPrefix(curEnvVar, envPrefix) {
			continue
		}
		env = append(env, curEnvVar)
	}
	// add gopherpack environment vars
	env = append(
		env,
		envValues...,
	)

	// run child process
	childProcess, err := os.StartProcess(
		filePath,
		os.Args,
		&os.ProcAttr{
			Dir:   dir,
			Env:   env,
			Files: files,
			Sys:   &syscall.SysProcAttr{},
		},
	)
	if err != nil {
		return nil, err
	}

	return childProcess, nil
}
