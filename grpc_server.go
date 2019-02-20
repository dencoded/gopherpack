package gopherpack

import (
	"errors"
	"net"
	"os"
	"os/signal"
	"syscall"
)

// GRPCServer specifies interface which gRPC server should implement to be controlled by gopherpack
// (https://godoc.org/google.golang.org/grpc#Server implements this interface)
type GRPCServer interface {
	Serve(net.Listener) error
	GracefulStop()
}

// ListenAndServeGRPC starts gRPC server on specified network and address.
// network parameter can be "tcp" or "unix"
// server parameter is where you pass ready to use gRPC-server (see https://godoc.org/google.golang.org/grpc#NewServer)
func ListenAndServeGRPC(network string, address string, server GRPCServer) error {
	// check if we are in main process
	if isMainProcess {
		return StartMainProcess()
	}

	// we are in a worker process
	if server == nil {
		return errors.New("nil server passed")
	}

	// setup runtime params
	if err := setupWorkerRuntime(); err != nil {
		return err
	}

	// announce listener
	l, err := getListenerWithSocketOptions(network, address)
	if err != nil {
		return err
	}

	// catch signals to do graceful shutdown
	go func() {
		// wait for signals to worker process
		sigChan := make(chan os.Signal, 1)
		signal.Notify(
			sigChan,
			syscall.SIGINT,
			syscall.SIGTERM,
			syscall.SIGQUIT,
		)
		sig := <-sigChan
		Logger.Printf("Worker process PID=%d received signal: %s. Shutdown gracefully\n", pid, sig)
		// check if we need to run custom logic before calling shutdown
		if OnServerShutdown != nil {
			func() {
				defer func() {
					panicErr := recover()
					Logger.Printf("Worker process PID=%d OnServerShutdown hook panicked: %s", pid, panicErr)
				}()
				OnServerShutdown()
			}()
		}
		// shutdown server gracefully
		server.GracefulStop()
	}()

	// start serving gRPC traffic
	return server.Serve(l)
}
