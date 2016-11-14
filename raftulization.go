package main

import (
	"fmt"
	"flag"
	"net"
	"net/http"
	"net/rpc"
	"log"
	"strconv"
	"time"
)

type TestRaft struct {
	State string
}

func (tr *TestRaft) SayHello(name string, reply *string) error {
	*reply = fmt.Sprintf("Hello %v, I'm %v", name, tr.State)
	return nil
}

func main() {

	var serverPort = flag.Int("s", 8080, "listen port")
	var clientPort = flag.Int("c", 8081, "connect port")
	flag.Parse()
	// server
	tr := TestRaft{"follower"}
	rpc.Register(&tr)
	rpc.HandleHTTP()
	l, e := net.Listen("tcp", ":" + strconv.Itoa(*serverPort))
	if e != nil {
		log.Fatal("listen error:" , e)
	}
	fmt.Printf("Listening on %v\n", serverPort)

	go http.Serve(l, nil)

	time.Sleep(time.Second * 2)

	// client
	client, err := rpc.DialHTTP("tcp", ":" + strconv.Itoa(*clientPort))
	if err != nil {
		log.Fatal("dialing:", err)
	}
	fmt.Printf("Connected to %v\n", clientPort)

	name := "elliot"
	res := ""
	err = client.Call("TestRaft.SayHello", name, &res)
	if err != nil {
		log.Fatal("say hello error:", err)
	}
	fmt.Printf("Response: %v\n", res)
}

