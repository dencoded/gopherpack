/*
Package gopherpack provides functionality to run network services written in Go on several CPU cores as pack of worker processes.
*/
package gopherpack

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/dencoded/gopherpack/system"
)

const (
	// this is how long new main process will wait before killing the previous main process
	prevMainProcessGraceInterval = 5 * time.Second

	logPrefix = "gopherpack: "
)

var (
	// OnSIGUSR2 is called in main process before starting executable upgrade process
	OnSIGUSR2 func()

	// OnServerShutdown is called in worker process before doing graceful server shutdown
	OnServerShutdown func()

	// Logger can be set to client's logging which should implements StdLogger,
	// default is Go's standard logger with output to stdout
	Logger StdLogger = log.New(os.Stdout, logPrefix, log.LstdFlags)
)

var (
	pid           = os.Getpid()
	isMainProcess = os.Getenv(envPPID) == ""
	workerCpuCore = os.Getenv(envCPUCore)
)

// IsMainProcess returns true if current process is not a worker
func IsMainProcess() bool {
	return isMainProcess
}

// GetWorkerCPUCoreNum returns number of CPU core currently used if it is a worker process
func GetWorkerCPUCoreNum() string {
	if isMainProcess {
		return "main process"
	}

	return workerCpuCore
}

// StartMainProcess starts main process and forks worker processes
func StartMainProcess() error {
	Logger.Printf("Main process PID=%d, starting up a pack..\n", pid)
	// run worker processes, one per each CPU core
	numCPU := runtime.NumCPU()
	workers := make([]*os.Process, numCPU)
	var err error
	for i := 0; i < numCPU; i++ {
		envVals := []string{
			fmt.Sprintf("%s=%d", envPPID, pid),  // to tell child that it is child
			fmt.Sprintf("%s=%d", envCPUCore, i), // to tell child on which core to settle on
		}
		if workers[i], err = forkProcess(envVals); err != nil {
			Logger.Printf("Could not start worker process. Error: %s\n", err)
		} else {
			Logger.Printf("Worker process PID=%d started on CPU core %d\n", workers[i].Pid, i)
		}
	}

	// terminate previos main process if needed (executable upgraded)
	if prevMainPIDStr := os.Getenv(envPrevPPID); prevMainPIDStr != "" {
		go func() {
			// let new main process and previous main process co-exist for some time
			time.Sleep(prevMainProcessGraceInterval)
			// send SIGTERM to previous main process
			prevMainPID, err := strconv.Atoi(prevMainPIDStr)
			if err != nil {
				Logger.Printf("Main process PID=%d could not parse previous PID: %s\n",
					pid, err)
			} else if prevProcess, err := os.FindProcess(prevMainPID); err != nil {
				Logger.Printf("Main process PID=%d could not find process for previous PID=%d: %s\n",
					pid, prevMainPID, err)
			} else if err := prevProcess.Signal(syscall.SIGTERM); err != nil {
				Logger.Printf("Main process PID=%d could not send SIGTERM to previous PID=%d: %s\n",
					pid, prevMainPID, err)
			}
		}()
	}

	// wait for signals to main process
	sigChan := make(chan os.Signal, 1)
	signal.Notify(
		sigChan,
		syscall.SIGINT,  // graceful shutdown
		syscall.SIGTERM, // graceful shutdown
		syscall.SIGQUIT, // graceful shutdown
		syscall.SIGUSR2, // upgrade executable
	)
	var sig os.Signal
	for {
		isExit := false
		sig = <-sigChan
		Logger.Printf("Main process PID=%d recivied signal: %s\n", pid, sig)
		switch sig {
		case syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT: // graceful shutdown:
			// propagate signal to workers and wait until they are done
			sendSignalToWorkers(workers, sig)
			isExit = true
		case syscall.SIGUSR2: // upgrade executable
			// call a hook if needed
			if OnSIGUSR2 != nil {
				func() {
					defer func() {
						panicErr := recover()
						Logger.Printf("Main process PID=%d OnSIGUSR2 hook panicked: %s", pid, panicErr)
					}()
					OnSIGUSR2()
				}()
			}
			Logger.Printf("Main process PID=%d starting new main process\n", pid)
			// send current main process PID via env var so new main process will know
			// which process to kill after successful start
			envValues := []string{
				fmt.Sprintf("%s=%d", envPrevPPID, pid),
			}
			if newMainProcess, err := forkProcess(envValues); err != nil {
				Logger.Printf("Main process PID=%d could not start new main process: %s\n",
					pid, err)
			} else {
				Logger.Printf("Main process PID=%d new main process PID=%d has started\n",
					pid, newMainProcess.Pid)
			}
		}
		if isExit {
			break
		}
	}

	// time for alpha gopher to exit
	return fmt.Errorf("signal received: %s", sig)
}

func sendSignalToWorkers(workers []*os.Process, sig os.Signal) {
	var wg sync.WaitGroup
	for _, worker := range workers {
		if worker == nil {
			continue
		}
		wg.Add(1)
		go func(p *os.Process) {
			defer wg.Done()
			if err := p.Signal(sig); err != nil {
				Logger.Printf("Could not send signal %s to worker process PID=%d. Error: %s\n",
					sig,
					p.Pid,
					err,
				)
			} else if pState, err := p.Wait(); err != nil {
				Logger.Printf("Waiting failed after sending signal %s to worker process PID=%d. Error: %s\n",
					sig,
					p.Pid,
					err,
				)
			} else {
				Logger.Printf("Worker process PID=%d exited with status: %s\n",
					p.Pid,
					pState,
				)
			}
		}(worker)
	}
	wg.Wait()
}

func setupWorkerRuntime() error {
	// set affinity to the number of core passed via env
	cpuCore, err := strconv.Atoi(workerCpuCore)
	if err != nil {
		return err
	}
	if err := system.SetAffinity(cpuCore); err != nil {
		Logger.Printf("Could not set affinity to CPU core %d: %s\n", cpuCore, err)
		return err
	}

	// tell runtime to use one core
	runtime.GOMAXPROCS(1)

	Logger.Printf("Starting worker PID=%d on CPU core %d\n", pid, cpuCore)

	return nil
}
