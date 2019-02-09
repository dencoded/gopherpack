package gopherpack

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

// ListenAndServeHttp starts HTTP server on specified network and address.
// network parameter can be "tcp" or "unix"
// TLS is supported by passing non nil server.TLSConfig
func ListenAndServeHttp(network string, address string, server *http.Server) error {
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
		Logger.Printf("Worker process PID=%d recivied signal: %s. Shutdown gracefully\n", pid, sig)
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
		if err := server.Shutdown(context.Background()); err != nil {
			Logger.Printf("Worker process PID=%d could not shutdown gracefully: %s\n", pid, err)
		}
	}()

	if server.TLSConfig != nil {
		Logger.Println("Using TLS")
		return server.ServeTLS(l, "", "")
	}

	return server.Serve(l)
}
