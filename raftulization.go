package main

import (
	"fmt"
	"flag"
	"net"
	"net/http"
	"net/rpc"
	"log"
	"strconv"
	"github.com/fresh4less/raftulization/raft"
	"os"
	"path"
)

func main() {

	var serverPort = flag.Int("s", 8080, "listen port")
	var clientPort = flag.Int("c", 8081, "connect port")
	var raftStatePath = flag.String("f", path.Join(os.TempDir(), "raftState.state"), "raft save state file path")
	flag.Parse()
	// server
	peers := []string{"", "127.0.0.1:" + strconv.Itoa(*clientPort)}
	applyCh := make(chan raft.ApplyMsg)
	rf := raft.MakeRaft(peers, 0, *raftStatePath, applyCh)

	rpc.Register(rf)
	rpc.HandleHTTP()
	l, e := net.Listen("tcp", ":" + strconv.Itoa(*serverPort))
	if e != nil {
		log.Fatal("listen error:" , e)
	}
	fmt.Printf("Listening on %v\n", serverPort)

	go http.Serve(l, nil)
	select{}
}

