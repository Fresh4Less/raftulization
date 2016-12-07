package main

import (
	"fmt"
	"github.com/fresh4less/raftulization/raft"
	"net"
	"net/http"
	"net/rpc"
	"strconv"
	"time"
)

// Interceptor acts as a proxy for RAFT to selectively drop RAFT RPCs
// Since RPCs don't get sender IP info, to distinguish between "incoming" and "outgoing" packets,
// for each remote peer we listen on a separate "source" and "remote" port
// This means that for each remote peer we create two RaftHandlers

type NetForwardInfo struct {
	SourceListenPort int

	RemoteListenPort int
	ForwardAddress   string
}

type Interceptor struct {
	listener     *net.Listener
	rpcServer    *rpc.Server
	raftHandlers []*RaftHandler
	matrixDisplay *PixelDisplay
	networkDisplays []*PixelDisplay
}

func NewInterceptor(eventListenPort int, sourceAddress string, forwardInfo []NetForwardInfo, matrixDisplay *PixelDisplay, networkDisplays []*PixelDisplay) *Interceptor {
	interceptor := Interceptor{}

	interceptor.rpcServer = rpc.NewServer()
	interceptor.rpcServer.Register(&interceptor)

	httpServer := http.NewServeMux()
	httpServer.Handle("/_goRPC_", interceptor.rpcServer)

	go http.ListenAndServe(":"+strconv.Itoa(eventListenPort), httpServer)

	fmt.Printf("Interceptor: Listening on %v\n", eventListenPort)

	for i, info := range forwardInfo {
		interceptor.raftHandlers = append(interceptor.raftHandlers,
			NewRaftHandler(&interceptor, info.SourceListenPort, info.ForwardAddress, i, true),
			NewRaftHandler(&interceptor, info.RemoteListenPort, sourceAddress, i, false))
	}

	interceptor.matrixDisplay = matrixDisplay
	interceptor.networkDisplays = networkDisplays

	return &interceptor
}

/*** RAFT events RPC ***/
func (interceptor *Interceptor) OnEvent(event raft.RaftEvent, reply *bool) error {
	*reply = interceptor.OnEventHandler(event)
	return nil
}

func (interceptor *Interceptor) OnEventHandler(event raft.RaftEvent) bool {
	switch event := event.(type) {
	case raft.StateUpdatedEvent:
		interceptor.updateStateDisplay(event)
	case raft.EntryCommittedEvent:
	case raft.SetElectionTimeoutEvent:
		//fmt.Printf("SetElectionTimeout: %v\n", event.Duration)
	case raft.SetHeartbeatTimeoutEvent:
		//fmt.Printf("SetHeartbeatTimeout: %v\n", event.Duration)
	case raft.AppendEntriesEvent:
		//if event.Outgoing {
			//fmt.Printf("AppendEntries: ->%v\n", event.Peer)
		//} else {
			//fmt.Printf("AppendEntries: <-%v\n", event.Peer)
		//}
		//animate(event.Args,event.Peer, event.Outgoing)
		//Term int
		//LeaderId int
		//PrevLogIndex int
		//PrevLogTerm int
		//Entries []Log
		//LeaderCommit int
	case raft.AppendEntriesResponseEvent:
		//if event.Outgoing {
			//fmt.Printf("AppendEntriesResponse: ->%v\n", event.Peer)
		//} else {
			//fmt.Printf("AppendEntriesResponse: <-%v\n", event.Peer)
		//}
	case raft.RequestVoteEvent:
		//if event.Outgoing {
			//fmt.Printf("RequestVote: ->%v\n", event.Peer)
		//} else {
			//fmt.Printf("RequestVote: <-%v\n", event.Peer)
		//}
	case raft.RequestVoteResponseEvent:
		//if event.Outgoing {
			//fmt.Printf("RequestVoteResponse: ->%v\n", event.Peer)
		//} else {
			//fmt.Printf("RequestVoteResponse: <-%v\n", event.Peer)
		//}
	default:
		fmt.Printf("Unexpected type %T\n", event)
	}
	return true
}

func (interceptor *Interceptor) updateStateDisplay(event raft.StateUpdatedEvent) {
	interceptor.matrixDisplay.Reset()
	//id TODO don't hardcode this
	interceptor.matrixDisplay.SetArea(0,0, MakeColorRect(2,2,MakeColor(255,0,0)))
	// voted for TODO
	//VotedFor int
	// received votes TODO: use id colors instead of just counting
	for i, voted := range event.ReceivedVotes {
		if voted {
			interceptor.matrixDisplay.Set(1,2+(i%4), MakeColor(255,200,0))
		}
	}
	// state
	interceptor.matrixDisplay.SetArea(0,6, MakeColorRect(2,2,StateColors[event.State]))
	// logs
	for i, log := range event.RecentLogs {
		if img, ok := log.Command.([][]Color); ok {
			//TODO: use last applied to display committed logs differently than uncommitted
			//LastApplied int
			//LogLength int
			interceptor.matrixDisplay.SetArea(i+8-len(event.RecentLogs), 2, MakeColorRect(1,2,averageColor(img)))
		}
	}
	// term
	interceptor.matrixDisplay.SetArea(5,0, MakeColorNumberChar(nthDigit(event.Term, 2), MakeColor(255,255,255), MakeColor(0,0,0)))
	interceptor.matrixDisplay.SetArea(5,3, MakeColorNumberChar(nthDigit(event.Term, 1), MakeColor(255,255,255), MakeColor(0,0,0)))
	interceptor.matrixDisplay.SetArea(5,6, MakeColorNumberChar(nthDigit(event.Term, 0), MakeColor(255,255,255), MakeColor(0,0,0)))
	interceptor.matrixDisplay.Draw()
}

// this just returns averages of the RGB channels, probably should make it return full saturation & value or something
func averageColor(colors [][]Color) Color {
	rTotal := uint32(0)
	gTotal := uint32(0)
	bTotal := uint32(0)
	count := uint32(0)
	for _, row := range colors {
		for _, color := range row {
			rTotal += color.GetRed()
			gTotal += color.GetGreen()
			bTotal += color.GetBlue()
			count++
		}
	}

	return MakeColor(rTotal/count, gTotal/count, bTotal/count)
}

func nthDigit(a, n int) int {
	for i := 0; i < n; i++ {
		a /= 10
	}
	return (a % 10)
}

type RaftHandler struct {
	interceptor   *Interceptor
	listener      *net.Listener
	rpcServer     *rpc.Server
	forwardClient *raft.UnreliableRpcClient
	peer          int
	outgoing      bool
}

func NewRaftHandler(interceptor *Interceptor, listenPort int, forwardAddress string, peer int, outgoing bool) *RaftHandler {

	rh := RaftHandler{}
	rh.interceptor = interceptor
	rh.peer = peer
	rh.outgoing = outgoing
	rh.rpcServer = rpc.NewServer()
	rh.rpcServer.RegisterName("Raft", &rh)

	httpServer := http.NewServeMux()
	httpServer.Handle("/_goRPC_", rh.rpcServer)

	go http.ListenAndServe(":"+strconv.Itoa(listenPort), httpServer)

	fmt.Printf("RaftHandler: Listening on %v\n", listenPort)

	rh.forwardClient = raft.NewUnreliableRpcClient(forwardAddress, 5, time.Second)

	return &rh
}

/*** RAFT RPCs **/
func (rh *RaftHandler) RequestVote(args raft.RequestVoteArgs, reply *raft.RequestVoteReply) error {
	rh.interceptor.OnEventHandler(raft.RequestVoteEvent{args, rh.peer, rh.outgoing})
	success := rh.forwardClient.Call("Raft.RequestVote", args, reply)
	if success {
		rh.interceptor.OnEventHandler(raft.RequestVoteResponseEvent{*reply, rh.peer, rh.outgoing})
	}
	return nil
}

func (rh *RaftHandler) AppendEntries(args raft.AppendEntriesArgs, reply *raft.AppendEntriesReply) error {
	rh.interceptor.OnEventHandler(raft.AppendEntriesEvent{args, rh.peer, rh.outgoing})
	success := rh.forwardClient.Call("Raft.AppendEntries", args, reply)
	if success {
		rh.interceptor.OnEventHandler(raft.AppendEntriesResponseEvent{*reply, rh.peer, rh.outgoing})
	}
	return nil
}

var StateColors = map[raft.ServerState]Color{
	raft.Follower: MakeColor(0,0,255),
	raft.Candidate: MakeColor(0,255,0),
	raft.Leader: MakeColor(255,0,0),
}
