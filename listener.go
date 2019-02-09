package gopherpack

import (
	"context"
	"errors"
	"net"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

func getListenerWithSocketOptions(network string, address string) (net.Listener, error) {
	listenConf := &net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var err, reuseAddrErr, reusePortErr, returnErr error
			err = c.Control(func(fd uintptr) {
				reuseAddrErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
				reusePortErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
			})

			errMsg := []string{}
			if err != nil {
				errMsg = append(errMsg, err.Error())
			}
			if reuseAddrErr != nil {
				errMsg = append(errMsg, reuseAddrErr.Error())
			}
			if reusePortErr != nil {
				errMsg = append(errMsg, reusePortErr.Error())
			}

			if len(errMsg) > 0 {
				returnErr = errors.New(strings.Join(errMsg, ";"))
			}

			return returnErr
		},
	}

	l, err := listenConf.Listen(context.Background(), network, address)
	if err != nil {
		Logger.Printf("Starting listener on %s\n", l.Addr())
	}

	return l, err
}
