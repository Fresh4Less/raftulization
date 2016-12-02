package main

import (
	"fmt"
	"flag"
	"net"
	"net/http"
	"net/rpc"
	"log"
	"strings"
	"strconv"
	"github.com/fresh4less/raftulization/raft"
	"os"
	"path"
	"errors"
)

type IpAddressList []string

func (ips *IpAddressList) String() string {
	return fmt.Sprint(*ips)
}

func (ips *IpAddressList) Set(value string) error {
	if len(*ips) > 0 {
		return errors.New("IpAddressList flag already set")
	}
	for _, ip := range strings.Split(value, ",") {
		//TODO: check if valid network address
		*ips = append(*ips, ip)
	}
	return nil
}
/*
.\raftulization.exe -s 8080 -c 127.0.0.1:8081,127.0.0.1:8082,127.0.0.1:8083 -f r1.state
.\raftulization.exe -s 8081 -c 127.0.0.1:8080,127.0.0.1:8082,127.0.0.1:8083 -f r2.state
.\raftulization.exe -s 8082 -c 127.0.0.1:8080,127.0.0.1:8081,127.0.0.1:8083 -f r3.state
.\raftulization.exe -s 8083 -c 127.0.0.1:8080,127.0.0.1:8081,127.0.0.1:8082 -f r4.state
*/

func main() {

	serverPort := flag.Int("s", 8080, "listen port")
	verbosity := flag.Int("v", 2, "verbosity--0: no logs, 1: commits and leader changes, 2: all state changes, 3: all messages, 4: all logs")
	peerAddresses := IpAddressList{}
	flag.Var(&peerAddresses, "c", "comma separated list of peer network addresses")

	var raftStatePath = flag.String("f", path.Join(os.TempDir(), "raftState.state"), "raft save state file path")
	flag.Parse()
	// server
	// add myself to peers
	peerAddresses = append(peerAddresses, ":" + strconv.Itoa(*serverPort))
	applyCh := make(chan raft.ApplyMsg)
	rf := raft.MakeRaft(peerAddresses, len(peerAddresses)-1, *raftStatePath, applyCh, *verbosity)

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

