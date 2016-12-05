package main

import(
	"fmt"
	"net"
	"net/http"
	"net/rpc"
	"strconv"
	"github.com/fresh4less/raftulization/raft"
	"time"
)

// Interceptor acts as a proxy for RAFT to selectively drop RAFT RPCs
// Since RPCs don't get sender IP info, to distinguish between "incoming" and "outgoing" packets,
// for each remote peer we listen on a separate "source" and "remote" port
// This means that for each remote peer we create two RaftHandlers

type NetForwardInfo struct {
	SourceListenPort int

	RemoteListenPort int
	ForwardAddress string
}

type Interceptor struct {
	listener *net.Listener
	rpcServer *rpc.Server
	raftHandlers []*RaftHandler
}

func NewInterceptor(eventListenPort int, sourceAddress string, forwardInfo []NetForwardInfo) *Interceptor {
	interceptor := Interceptor{}

	interceptor.rpcServer = rpc.NewServer()
	interceptor.rpcServer.Register(&interceptor)

	httpServer := http.NewServeMux()
	httpServer.Handle("/_goRPC_", interceptor.rpcServer)
	
	go http.ListenAndServe(":" + strconv.Itoa(eventListenPort), httpServer)
	
	fmt.Printf("Interceptor: Listening on %v\n", eventListenPort)

	for i, info := range forwardInfo {
		interceptor.raftHandlers = append(interceptor.raftHandlers,
			NewRaftHandler(&interceptor, info.SourceListenPort, info.ForwardAddress, i, true),
			NewRaftHandler(&interceptor, info.RemoteListenPort, sourceAddress, i, false))
	}

	return &interceptor
}

/*** RAFT events RPC ***/
func (interceptor *Interceptor) OnEvent(event RaftEvent, reply *bool) error {
	*reply = interceptor.OnEventHandler(event)
	return nil
}

func (interceptor *Interceptor) OnEventHandler(event RaftEvent) bool {
	switch event := event.(type) {
	case AppendEntriesEvent:
		if event.Outgoing {
			fmt.Printf("AppendEntries: ->%v\n", event.Peer)
		} else {
			fmt.Printf("AppendEntries: <-%v\n", event.Peer)
		}
		//animate(event.Args,event.Peer, event.Outgoing)
		//Term int
		//LeaderId int
		//PrevLogIndex int
		//PrevLogTerm int
		//Entries []Log
		//LeaderCommit int
	case AppendEntriesResponseEvent:
		if event.Outgoing {
			fmt.Printf("AppendEntriesResponse: ->%v\n", event.Peer)
		} else {
			fmt.Printf("AppendEntriesResponse: <-%v\n", event.Peer)
		}
	case RequestVoteEvent:
		if event.Outgoing {
			fmt.Printf("RequestVote: ->%v\n", event.Peer)
		} else {
			fmt.Printf("RequestVote: <-%v\n", event.Peer)
		}
	case RequestVoteResponseEvent:
		if event.Outgoing {
			fmt.Printf("RequestVoteResponse: ->%v\n", event.Peer)
		} else {
			fmt.Printf("RequestVoteResponse: <-%v\n", event.Peer)
		}
	case LogUpdatedEvent:
	default:
		fmt.Printf("Unexpected type %T\n", event)
	}
	return true
}

type RaftHandler struct {
	interceptor *Interceptor
	listener *net.Listener
	rpcServer *rpc.Server
	forwardClient *raft.UnreliableRpcClient
	peer int
	outgoing bool
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

	go http.ListenAndServe(":" + strconv.Itoa(listenPort), httpServer)

	fmt.Printf("RaftHandler: Listening on %v\n", listenPort)

	rh.forwardClient = raft.NewUnreliableRpcClient(forwardAddress, 5, time.Second)

	return &rh
}

/*** RAFT RPCs **/
func (rh *RaftHandler) RequestVote(args raft.RequestVoteArgs, reply *raft.RequestVoteReply) error {
	rh.interceptor.OnEventHandler(RequestVoteEvent{args, rh.peer, rh.outgoing})
	success := rh.forwardClient.Call("Raft.RequestVote", args, reply)
	if success {
		rh.interceptor.OnEventHandler(RequestVoteResponseEvent{*reply, rh.peer, rh.outgoing})
	}
	return nil
}

func (rh *RaftHandler) AppendEntries(args raft.AppendEntriesArgs, reply *raft.AppendEntriesReply) error {
	rh.interceptor.OnEventHandler(AppendEntriesEvent{args, rh.peer, rh.outgoing})
	success := rh.forwardClient.Call("Raft.AppendEntries", args, reply)
	if success {
		rh.interceptor.OnEventHandler(AppendEntriesResponseEvent{*reply, rh.peer, rh.outgoing})
	}
	return nil
}
