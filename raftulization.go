package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/fresh4less/raftulization/raft"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"time"
	"os"
	"path"
	"strconv"
	"strings"
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

func main() {
	if len(os.Args) <= 1 {
		fmt.Printf("usage: raftulization raft|intercept [options]\n")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "raft":
		doRaft()
	case "intercept":
		doIntercept()
	default:
		fmt.Printf("usage: raftulization raft|intercept [options]\n")
		os.Exit(1)
	}
}

func doRaft() {
	raftFlagSet := flag.NewFlagSet("", flag.ExitOnError)

	serverPort := raftFlagSet.Int("s", 8080, "listen port")
	eventAddress := raftFlagSet.String("e", "127.0.0.1:10000", "raft interceptor event address")
	verbosity := raftFlagSet.Int("v", 2, "verbosity--0: no logs, 1: commits and leader changes, 2: all state changes, 3: all messages, 4: all logs")
	peerAddresses := IpAddressList{}
	raftFlagSet.Var(&peerAddresses, "c", "comma separated list of peer network addresses")

	var raftStatePath = raftFlagSet.String("f", path.Join(os.TempDir(), "raftState.state"), "raft save state file path")
	fmt.Printf("Saving state at %v\n", *raftStatePath)

	raftFlagSet.Parse(os.Args[2:])
	// server
	// add myself to peers
	peerAddresses = append(peerAddresses, ":"+strconv.Itoa(*serverPort))
	applyCh := make(chan raft.ApplyMsg)
	eventCh := make(chan raft.RaftEvent)
	rf := raft.MakeRaft(peerAddresses, len(peerAddresses)-1, *raftStatePath, applyCh, *verbosity, eventCh)

	rpc.Register(rf)
	rpc.HandleHTTP()
	l, e := net.Listen("tcp", ":"+strconv.Itoa(*serverPort))
	if e != nil {
		log.Fatal("listen error:", e)
	}
	fmt.Printf("Listening on %v\n", *serverPort)

	go http.Serve(l, nil)

	eventClient := raft.NewUnreliableRpcClient(*eventAddress, 5, time.Second)

	for(true) {
		select {
			//we don't strictly need this event, because whenever LastApplied is incremented we send a StateUpdatedEvent
			//however, it's useful enough to know when something is committed that we send these events anyway
			case applyMsg := <-applyCh:
				go func() {
					eventCh<-raft.EntryCommittedEvent{applyMsg}
				}()
			case event := <-eventCh:
				go func() {
					eventClient.Call("Interceptor.OnEvent", &event, nil)
				}()
		}
	}
}

type NetForwardInfoList []NetForwardInfo

func (infoList *NetForwardInfoList) String() string {
	return fmt.Sprint(*infoList)
}

func (infoList *NetForwardInfoList) Set(value string) error {
	if len(*infoList) > 0 {
		return errors.New("NetForwardInfoList flag already set")
	}
	for _, info := range strings.Split(value, ",") {
		parts := strings.Split(info, "~")
		sourceListenPort, err := strconv.Atoi(parts[0])
		if err != nil {
			return err
		}
		remoteListenPort, err := strconv.Atoi(parts[1])
		if err != nil {
			return err
		}
		//TODO: check if valid network address
		*infoList = append(*infoList, NetForwardInfo{sourceListenPort, remoteListenPort, parts[2]})
	}
	return nil
}

func doIntercept() {
	interceptFlagSet := flag.NewFlagSet("", flag.ExitOnError)
	eventListenPort := interceptFlagSet.Int("e", 10000, "Event listen port")
	sourceAddress := interceptFlagSet.String("s", "127.0.0.1:8000", "RAFT source address")
	pixelsEnabled := interceptFlagSet.Bool("p", false, "Enable pixel displays")

	forwardInfo := NetForwardInfoList{}
	interceptFlagSet.Var(&forwardInfo, "f", "comma separated list of forward info in form inPort~outPort~remoteAddress")
	interceptFlagSet.Parse(os.Args[2:])

	var neopixelDisplay PixelDisplay
	if *pixelsEnabled {
		neopixelDisplay = NewNeopixelDisplay(18, 64+30+20, 255)
	} else {
		neopixelDisplay = &FakeDisplay{64+30+20}
	}

	matrixDisplay := NewPixelDisplayView(neopixelDisplay, 0, 8, 8, false)
	networkDisplays := []*PixelDisplayView{
		NewPixelDisplayView(neopixelDisplay, 64, 1, 30, false),
		NewPixelDisplayView(neopixelDisplay, 64+30, 1, 20, false),
	}
	for i := 0; i < 64; i++ {
		neopixelDisplay.Set(i, MakeColor(255,0,0))
	}
	for i := 64; i < 64+30; i++ {
		neopixelDisplay.Set(i, MakeColor(0,255,0))
	}
	for i := 64+30; i < 64+30+20; i++ {
		neopixelDisplay.Set(i, MakeColor(0,0,255))
	}
	neopixelDisplay.Show()
	/*matrixDisplay.SetArea(0,0,MakeColorRect(8,8,MakeColor(255,0,0)))*/
	/*matrixDisplay.Draw()*/
	/*networkDisplays[0].SetArea(0,0,MakeColorRect(30,1,MakeColor(0,255,0)))*/
	/*networkDisplays[0].Draw()*/
	/*networkDisplays[1].SetArea(0,0,MakeColorRect(30,1,MakeColor(0,0,255)))*/
	/*networkDisplays[1].Draw()*/

	NewInterceptor(*eventListenPort, *sourceAddress, forwardInfo, matrixDisplay, networkDisplays)
	select {}
}

/*
Port convention--90ab: a=senderId, b=recipientId
1.1.1.1
.\raftulization.exe intercept -e 10000 -s 127.0.0.1:8000 -f 9012~9021~2.2.2.2:8000,9013~9031~3.3.3.3:8000
.\raftulization.exe raft -s 8000 -c 127.0.0.1:9012,127.0.0.1:9013,4.4.4.4:9014,5.5.5.5:9015 -f r1.state
2.2.2.2
.\raftulization.exe intercept -e 10000 -s 127.0.0.1:8000 -f 9023~9032~3.3.3.3:8000,9024~9042~4.4.4.4:8000
.\raftulization.exe raft -s 8000 -c 1.1.1.1:9021,127.0.0.1:9023,127.0.0.1:9024,5.5.5.5:9025 -f r2.state
3.3.3.3
.\raftulization.exe intercept -e 10000 -s 127.0.0.1:8000 -f 9034~9043~4.4.4.4:8000,9035~9053~5.5.5.5:8000
.\raftulization.exe raft -s 8000 -c 1.1.1.1:9031,2.2.2.2:9032,127.0.0.1:9034,127.0.0.1:9035 -f r3.state
4.4.4.4
.\raftulization.exe intercept -e 10000 -s 127.0.0.1:8000 -f 9045~9054~5.5.5.5:8000,9041~9014~1.1.1.1:8000
.\raftulization.exe raft -s 8000 -c 127.0.0.1:9041,2.2.2.2:9042,3.3.3.3:9043,127.0.0.1:9045 -f r4.state
5.5.5.5
.\raftulization1exe intercept -e 10000 -s 127.0.0.1:8000 -f 9051~9015~1.1.1.1:8000,9052~9025~2.2.2.2:8000
.\raftulization.exe raft -s 8000 -c 127.0.0.1:9051,127.0.0.1:9052,3.3.3.3:9053,4.4.4.4:9054 -f r5.state

local testing version (instead of different Ips, different different 800x digit
1
.\raftulization.exe intercept -e 10001 -s 127.0.0.1:8001 -f 9012~9021~127.0.0.1:8002,9013~9031~127.0.0.1:8003
.\raftulization.exe raft -s 8001 -c 127.0.0.1:9012,127.0.0.1:9013,127.0.0.1:9014,127.0.0.1:9015 -f r1.state
2
.\raftulization.exe intercept -e 10002 -s 127.0.0.1:8002 -f 9023~9032~127.0.0.1:8003,9024~9042~127.0.0.1:8004
.\raftulization.exe raft -s 8002 -c 127.0.0.1:9021,127.0.0.1:9023,127.0.0.1:9024,127.0.0.1:9025 -f r2.state
3
.\raftulization.exe intercept -e 10003 -s 127.0.0.1:8003 -f 9034~9043~127.0.0.1:8004,9035~9053~127.0.0.1:8005
.\raftulization.exe raft -s 8003 -c 127.0.0.1:9031,127.0.0.1:9032,127.0.0.1:9034,127.0.0.1:9035 -f r3.state
4
.\raftulization.exe intercept -e 10004 -s 127.0.0.1:8004 -f 9045~9054~127.0.0.1:8005,9041~9014~127.0.0.1:8001
.\raftulization.exe raft -s 8004 -c 127.0.0.1:9041,127.0.0.1:9042,127.0.0.1:9043,127.0.0.1:9045 -f r4.state
5
.\raftulization1exe intercept -e 10005 -s 127.0.0.1:8005 -f 9051~9015~127.0.0.1:8001,9052~9025~127.0.0.1:8002
.\raftulization.exe raft -s 8005 -c 127.0.0.1:9051,127.0.0.1:9052,127.0.0.1:9053,127.0.0.1:9054 -f r5.state



.\raftulization.exe raft -s 8000 -c 127.0.0.1:8081,127.0.0.1:8082,127.0.0.1:8083 -f r1.state
.\raftulization.exe raft -s 8081 -c 127.0.0.1:8080,127.0.0.1:8082,127.0.0.1:8083 -f r2.state
.\raftulization.exe raft -s 8082 -c 127.0.0.1:8080,127.0.0.1:8081,127.0.0.1:8083 -f r3.state
.\raftulization.exe raft -s 8083 -c 127.0.0.1:8080,127.0.0.1:8081,127.0.0.1:8082 -f r4.state

.\raftulization.exe intercept -e 10001 -s 127.0.0.1:8001 -f 9012~9021~127.0.0.1:8002
.\raftulization.exe raft -s 8001 -e 127.0.0.1:10001 -c 127.0.0.1:9012 -f r1.state

.\raftulization.exe raft -s 8002 -c 127.0.0.1:9021 -f r2.state
3
*/
