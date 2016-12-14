package main

import (
	"encoding/gob"
	"fmt"
	"github.com/fresh4less/raftulization/raft"
	"net"
	"net/http"
	"net/rpc"
	"math/rand"
	"strconv"
	"time"
)

// Interceptor acts as a proxy for RAFT to selectively drop RAFT RPCs
// Since RPCs don't get sender IP info, to distinguish between "incoming" and "outgoing" packets,
// for each remote peer we listen on a separate "source" and "remote" port
// This means that for each remote peer we create two RaftHandlers

const NetworkForwardDelay = time.Millisecond * 700

type NetForwardInfo struct {
	SourceListenPort int

	RemoteListenPort int
	ForwardAddress   string
}

type SetPixelCommand struct {
	X, Y int
	PixelColor Color
}

type Interceptor struct {
	listener        *net.Listener
	rpcServer       *rpc.Server
	raftHandlers    []*RaftHandler

	matrixDisplay   *PixelDisplayView
	networkDisplays []*PixelDisplayView
	interactiveDisplay *PixelDisplayView

	raftStateScreen ColorFrame
	imageScreen ColorFrame
	matrixMultiFrameView *MultiFrameView
}

func NewInterceptor(eventListenPort int, sourceAddress string, forwardInfo []NetForwardInfo, matrixDisplay *PixelDisplayView, networkDisplays []*PixelDisplayView, interactiveDisplay *PixelDisplayView) *Interceptor {
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
	interceptor.interactiveDisplay = interactiveDisplay

	interceptor.raftStateScreen = MakeColorFrame(8,8,MakeColor(0,0,0))
	interceptor.imageScreen = MakeColorFrame(8,8,MakeColor(255,255,255))

	interceptor.matrixMultiFrameView = NewMultiFrameView(matrixDisplay)
	
	//begin animation cycle
	interceptor.matrixMultiFrameView.CycleFrames(
		[]*ColorFrame{&interceptor.imageScreen, &interceptor.raftStateScreen},
		[]time.Duration{time.Second*2, time.Second*5},
		[]FrameTransition{Slide, Slide})

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
		interceptor.onStateUpdated(event)
	case raft.EntryCommittedEvent:
		interceptor.onEntryCommitted(event)
	case raft.SetElectionTimeoutEvent:
		//fmt.Printf("SetElectionTimeout: %v\n", event.Duration)
	case raft.SetHeartbeatTimeoutEvent:
		//fmt.Printf("SetHeartbeatTimeout: %v\n", event.Duration)
	case raft.AppendEntriesEvent:
		colors := MakeColorFrame(1,1 + len(event.Args.Entries), MakeColor(0,0,0))
		colors[0][len(colors[0])-1] = MakeColor(255,0,0)
		for i := 0; i < len(colors[0])-1; i++ {
			colors[0][i] = event.Args.Entries[i].Command.(SetPixelCommand).PixelColor
		}

		animation := MakeMovingSegmentAnimation(colors, interceptor.networkDisplays[0].Width)

		go interceptor.networkDisplays[event.Peer].DrawAnimation(animation, calcFps(len(animation)))

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

func calcFps(frameCount int) float32 {
	return float32(1000*frameCount)/float32(NetworkForwardDelay/time.Millisecond)
}


func (interceptor *Interceptor) onStateUpdated(event raft.StateUpdatedEvent) {
	interceptor.raftStateScreen.SetAll(MakeColor(0,0,0))
	//id TODO don't hardcode this
	interceptor.raftStateScreen.SetRect(0, 0, MakeColorFrame(2, 2, MakeColor(255, 0, 0)), Error)
	// voted for TODO
	//VotedFor int
	// received votes TODO: use id colors instead of just counting
	for i, voted := range event.ReceivedVotes {
		if voted {
			interceptor.raftStateScreen.Set(2+(i%4), 1, MakeColor(255, 200, 0), Error)
		}
	}
	// state
	interceptor.raftStateScreen.SetRect(6, 0, MakeColorFrame(2, 2, StateColors[event.State]), Error)
	// logs
	for i, log := range event.RecentLogs {
		if setPixelCommand, ok := log.Command.(SetPixelCommand); ok {
			//TODO: use last applied to display committed logs differently than uncommitted
			//LastApplied int
			//LogLength int
			interceptor.raftStateScreen.SetRect(i+8-len(event.RecentLogs), 2, MakeColorFrame(1, 2, setPixelCommand.PixelColor), Error)
		}
	}
	// term
	interceptor.raftStateScreen.SetRect(0, 5, MakeColorNumberChar(nthDigit(event.Term, 2), MakeColor(255, 255, 255), MakeColor(0, 0, 0)), Error)
	interceptor.raftStateScreen.SetRect(3, 5, MakeColorNumberChar(nthDigit(event.Term, 1), MakeColor(255, 255, 255), MakeColor(0, 0, 0)), Error)
	interceptor.raftStateScreen.SetRect(6, 5, MakeColorNumberChar(nthDigit(event.Term, 0), MakeColor(255, 255, 255), MakeColor(0, 0, 0)), Error)
	interceptor.matrixMultiFrameView.UpdateFrame(&interceptor.raftStateScreen)
}

func (interceptor *Interceptor) onEntryCommitted(event raft.EntryCommittedEvent) {
	interceptor.imageScreen.SetRect(0,0, event.State.(ColorFrame), Error)
	interceptor.matrixMultiFrameView.UpdateFrame(&interceptor.imageScreen)
	//interceptor.matrixMultiFrameView.CycleFrames(
		//[]*ColorFrame{&interceptor.imageScreen, &interceptor.raftStateScreen},
		//[]time.Duration{time.Second*5, time.Second*2},
		//[]FrameTransition{Slide, Slide})
}

// moves horizontally only
func MakeMovingSegmentAnimation(colors ColorFrame, length int) []ColorFrame {
	frameCount := len(colors[0])+ length-1
	frames := make([]ColorFrame, frameCount)
	for frame := 0; frame < frameCount; frame++ {
		frames[frame] = MakeColorFrame(length, 1, MakeColor(0,0,0))
		beginIndex := frame - (len(colors[0])-1)
		for i, color := range colors[0] {
			frames[frame].Set(beginIndex+i,0, color, Clip)
		}
	}
	return frames
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
	go func() {
		for true {
			time.Sleep(time.Duration(time.Duration(5+rand.Intn(5))*time.Second))
			reply := raft.StartReply{}
			success := rh.forwardClient.Call("Raft.Start", raft.StartArgs{SetPixelCommand{rand.Intn(8),rand.Intn(8),MakeColorHue(uint32(rand.Int31n(256)))}}, &reply)
			if success {
				fmt.Printf("Start hello: %v\n", reply)
			} else {
				fmt.Printf("err\n")
			}
		}
	}()

	return &rh
}

/*** RAFT RPCs **/
func (rh *RaftHandler) RequestVote(args raft.RequestVoteArgs, reply *raft.RequestVoteReply) error {
	rh.interceptor.OnEventHandler(raft.RequestVoteEvent{args, rh.peer, rh.outgoing})
	time.Sleep(NetworkForwardDelay)
	success := rh.forwardClient.Call("Raft.RequestVote", args, reply)
	if success {
		rh.interceptor.OnEventHandler(raft.RequestVoteResponseEvent{*reply, rh.peer, rh.outgoing})
		time.Sleep(NetworkForwardDelay)
	}
	return nil
}

func (rh *RaftHandler) AppendEntries(args raft.AppendEntriesArgs, reply *raft.AppendEntriesReply) error {
	rh.interceptor.OnEventHandler(raft.AppendEntriesEvent{args, rh.peer, rh.outgoing})
	time.Sleep(NetworkForwardDelay)
	success := rh.forwardClient.Call("Raft.AppendEntries", args, reply)
	if success {
		rh.interceptor.OnEventHandler(raft.AppendEntriesResponseEvent{*reply, rh.peer, rh.outgoing})
		time.Sleep(NetworkForwardDelay)
	}
	return nil
}

func (rh *RaftHandler) Start(args raft.StartArgs, reply *raft.StartReply) error {
	time.Sleep(NetworkForwardDelay)
	rh.interceptor.OnEventHandler(raft.StartEvent{args, rh.peer, rh.outgoing})
	success := rh.forwardClient.Call("Raft.Start", args, reply)
	if success {
		rh.interceptor.OnEventHandler(raft.StartResponseEvent{*reply, rh.peer, rh.outgoing})
		time.Sleep(NetworkForwardDelay)
	}
	return nil
}

var StateColors = map[raft.ServerState]Color{
	raft.Follower:  MakeColor(0, 0, 255),
	raft.Candidate: MakeColor(0, 255, 0),
	raft.Leader:    MakeColor(255, 0, 0),
}

func init() {
	gob.Register(SetPixelCommand{})
	gob.Register(MakeColorFrame(0,0,0))
}
