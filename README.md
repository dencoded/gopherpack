gopherpack
=======

Using workers based process model to run network servers written in Go
----------------------------------------------------------------------

Why is it `gopherpack`? It is a pack of gophers pulling your sled with network server.

The `gopherpack` package provides a way to run your network server as a main process and bunch of worker-processes (who are child-processes of main process).

Main process (aka alpha-gopher) controls worker processes (the pack members). Its responsibilities are:

- start main process and listen for system signals
- launch worker processes - one per each CPU core, sets CPU affinity of each worker to the needed core
- stop workers on signals `SIGINT`, `SIGTERM` or `SIGQUIT` and do exit
- reload (aka upgrade executable) workers and itself on `SIGUSR2` signal
- there is no any network server in main process (!)

Worker process - this is where your network server lives and handles connections. Worker process does several things:

- sets its `GOMAXPROCS=1` to have only one system thread to be used
- serves and listens network with using socket option `SO_REUSEPORT`
- sets number of file descriptors to possible maximum via `RLIMIT_NOFILE` sys-call
- listens for signals from main process and does graceful shutdown when main process asks to stop

This approach allows you to run network server as several processes listening the same port and gives you several accept/handle connection loops instead of one.

Also, using `SO_REUSEPORT` brings highly efficient distribution of network traffic (done by OS-kernel) over your worker processes listening on the same port. You can handle more concurrent connections.

Isolating each worker on single CPU core helps scheduler to do work more efficiently.

Type of network servers
-----------------------
- HTTP-server, see function `ListenAndServeHttp` (with TLS support)
- TCP-server, see function `ListenAndServeTCP` (with TLS support)
- gRPC-server - see function `ListenAndServeGRPC` (with TLS support)

Attaching gopherpack to your logging
------------------------------------
By default gopherpack will be writing logs to stdout using standard Go's logger.
To use some custom logger you will need to set exported var `ghoperpack.Logger`. Your logger should implement this standard logger interface (like logrus does):
```go
type StdLogger interface {
	Print(...interface{})
	Printf(string, ...interface{})
	Println(...interface{})

	Fatal(...interface{})
	Fatalf(string, ...interface{})
	Fatalln(...interface{})

	Panic(...interface{})
	Panicf(string, ...interface{})
	Panicln(...interface{})
}
```

Installation
------------
```bash
go get -u github.com/dencoded/gopherpack
```

Code examples
-------------

Integration with existing code base is pretty straightforward.

HTTP-server example:
```go
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
		Handler: mux, // this can be any http.Handler, func or mux with complex routing
	}

	// start listener as part of worker process
	// "tcp" and "unix" are the networks supported
	log.Fatalln(gopherpack.ListenAndServeHttp("tcp", "localhost:8778", server))
}
```

Also, if you need to do some significant initialization work before start listening and you don't want to have this logic in main process:
```go
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/dencoded/gopherpack"
)

func main() {
	// Main process initialization goes here
	if gopherpack.IsMainProcess() {
		// this is blocking call
		log.Fatalln(gopherpack.StartMainProcess())
	}

	// Worker process initialization goes here
	childID := "worker-" + gopherpack.GetWorkerCPUCoreNum()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		io.WriteString(w, fmt.Sprintf("Hello, world from %s!\n", childID))
	})

	// set server.TLSConfig if you want tls support
	// and any other http.Server fields, i.e. read/write timeouts
	server := &http.Server{
		Handler: mux, // this can be any http.Handler, func or mux with complex routing
	}

	// start listener as part of worker process
	// "tcp" and "unix" are the networks supported
	log.Fatalln(gopherpack.ListenAndServeHttp("tcp", "localhost:8778", server))
}
``` 
TCP-server example:
```go
package main

import (
	"io"
	"log"
	"net"

	"github.com/dencoded/gopherpack"
)

var childID = "worker-" + gopherpack.GetWorkerCPUCoreNum()

func handler(c net.Conn) {
	c.Write([]byte("Welcome to echo-service from " + childID + ":\n"))
	io.Copy(c, c)
	c.Close()
}

func main() {
	log.Fatalln(
		gopherpack.ListenAndServeTCP(
			"tcp",
			"localhost:8777",
			nil,     // pass here *tls.Config if you need TLS support
			handler, // this is our connection handler
		),
	)
}
```
gRPC server example (i.e. migrating https://github.com/grpc/grpc-go/tree/master/examples/helloworld to run with `gopherpack`):
```go
package main

import (
        "context"
        "log"

        "google.golang.org/grpc"
        pb "google.golang.org/grpc/examples/helloworld/helloworld"
        "google.golang.org/grpc/reflection"

        "github.com/dencoded/gopherpack"
)

const (
        port = ":50051"
)

// server is used to implement helloworld.GreeterServer.
type server struct{}

var childID = "worker-" + gopherpack.GetWorkerCPUCoreNum()

// SayHello implements helloworld.GreeterServer
func (s *server) SayHello(ctx context.Context, in *pb.HelloRequest) (*pb.HelloReply, error) {
        log.Printf("Received: %v", in.Name)
        return &pb.HelloReply{Message: "Hello " + in.Name + " from " + childID}, nil
}

func main() {
        s := grpc.NewServer()
        pb.RegisterGreeterServer(s, &server{})
        // Register reflection service on gRPC server.
        reflection.Register(s)
        if err := gopherpack.ListenAndServeGRPC("tcp", port, s); err != nil {
                log.Fatalf("failed to serve: %v", err)
        }
}
```

NOTE:
- on Mac OS:
  - CPU-affinity API is not exposed so worker process gets placed on CPU core by OS
  - network load distribution over worker processes might look not very efficient
- Windows is not supported