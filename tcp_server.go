package gopherpack

import (
	"crypto/tls"
	"net"
)

// ListenAndServeTCP starts TCP server on specified network and address.
// network parameter can be "tcp" or "unix"
// TLS is supported by passing non nil tlsConfig
// handler parameter is a callback function called as Go-routine when new connection accepted
func ListenAndServeTCP(network string, address string, tlsConfig *tls.Config, handler func(net.Conn)) error {
	// check if we are in main process
	if isMainProcess {
		return StartMainProcess()
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
	defer l.Close()

	// check if we need to do TLS
	if tlsConfig != nil {
		Logger.Println("Using TLS")
		l = tls.NewListener(l, tlsConfig)
	}

	// start accept/handle connection loop
	for {
		conn, err := l.Accept()
		if err != nil {
			Logger.Printf("Worker process PID=%d accept connection error: %s", pid, err)
			continue
		}
		Logger.Printf("New connection accepted from %s/%s\n", conn.RemoteAddr().Network(), conn.RemoteAddr().String())
		go handler(conn)
	}
}
