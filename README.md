gopherpack
=======

Running network servers written in Go with using workers based process model
----------------------------------------------------------------------------

Why is it `gopherpack`? It is a pack of gophers pulling your sled with network server.

The `gopherpack` package provides a way to run you network server as a main process and bunch of worker-processes (who are child-processes of main process).

Main process (aka alpha-gopher) controls worker processes (the pack members). Its responsibilities are:

- start main process and listen for system signals
- launch worker process - one per each CPU core
- stop workers on signals `SIGINT`, `SIGTERM` and `SIGQUIT` and exit
- reload (aka upgrade executable) workers and itself on `SIGUSR2`
- there is no any network server in main process (!)

Worker process - is where your network server lives and handles connections. Worker process does several things:

- forces where its Go-routines can be scheduled via setting `GOMAXPROCS=1` and changing its affinity to one CPU core (core number is passed by main process)
- serves and listens network with using socket option `SO_REUSEPORT`
- listens for signals from main process and does graceful shutdown of network server when main process asks to stop

This approach allows you to run network server as several processes on system and gives you several accept/handle connection loops (instead of one).

Also, using `SO_REUSEPORT` brings highly efficient distribution of network traffic (done by OS-kernel) over you worker processes listening the same port.

Isolating each worker on single CPU helps Go-scheduler to work efficiently.

Installation
------------
```bash
go get -u github.com/dencoded/gopherpack
```

Code example
------------

Integration with existing code base is pretty straightforward:
```
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/dencoded/gopherpack"
)

var childID = "worker-" + gopherpack.GetWorkerCPUCoreNum()

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		io.WriteString(w, fmt.Sprintf("Hello, world from %s!\n", childID))
	})

	// set server.TLSConfig if you want tls support
	// and any other http.Server fields, i.e. read/write timeouts
	server := &http.Server{
		Handler: mux, // this can be any http.Handler, func ur mux with complex routing
	}

	// "tcp" and "unix" are the networks supported
	log.Fatalln(gopherpack.ListenAndServeHttp("tcp", "localhost:8778", server))
}
```

NOTE:

- raw TCP network servers are not supported yet
- on Mac OS there is no CPU affinity and load distribution over worker process might look strange (but main and worker process work)