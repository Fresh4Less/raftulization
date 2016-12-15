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

const NetworkForwardDelay = time.Millisecond * 1500

//colors
var StateColors = map[raft.ServerState]Color{
	raft.Follower:  MakeColor(0, 0, 255),
	raft.Candidate: MakeColor(0, 255, 0),
	raft.Leader:    MakeColor(255, 0, 0),
}

var RpcColors = map[string]Color {
	"AppendEntries": MakeColor(255,0,0),
	"AppendEntriesResponse": MakeColor(255,165,0),
	"RequestVote": MakeColor(0,0,255),
	"RequestVoteResponse": MakeColor(0,165,255),
	"StartEvent": MakeColor(0,255,0),
	"StartEventResponse": MakeColor(165,255,0),
}

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
	networkMultiAnimViews []*MultiAnimationView
	matrixMultiFrameView *MultiFrameView
	idColor Color
	
	networksEnabled []bool
	
	s1Data	chan int
	s2Data	chan int
	interactiveChans	*InteractiveChannels
}

func NewInterceptor(eventListenPort int, sourceAddress string, forwardInfo []NetForwardInfo, matrixDisplay *PixelDisplayView, networkDisplays []*PixelDisplayView, interactiveDisplay *PixelDisplayView, s1Data, s2Data chan int, interactiveChans *InteractiveChannels) *Interceptor {
	interceptor := Interceptor{}

	interceptor.rpcServer = rpc.NewServer()
	interceptor.rpcServer.Register(&interceptor)

	httpServer := http.NewServeMux()
	httpServer.Handle("/_goRPC_", interceptor.rpcServer)

	go http.ListenAndServe(":"+strconv.Itoa(eventListenPort), httpServer)

	fmt.Printf("Interceptor: Listening on %v\n", eventListenPort)

	for i, info := range forwardInfo {
		interceptor.raftHandlers = append(interceptor.raftHandlers,
			NewRaftHandler(&interceptor, info.SourceListenPort, info.ForwardAddress, i, false),
			NewRaftHandler(&interceptor, info.RemoteListenPort, sourceAddress, i, true))
	}

	interceptor.matrixDisplay = matrixDisplay
	interceptor.networkDisplays = networkDisplays
	interceptor.interactiveDisplay = interactiveDisplay

	interceptor.raftStateScreen = MakeColorFrame(8,8,MakeColor(0,0,0))
	interceptor.imageScreen = MakeColorFrame(8,8,MakeColor(255,255,255))

	for _, networkDisplay := range interceptor.networkDisplays {
		interceptor.networkMultiAnimViews = append(interceptor.networkMultiAnimViews, NewMultiAnimationView(networkDisplay, Add, Error))
	}

	interceptor.matrixMultiFrameView = NewMultiFrameView(matrixDisplay)
	
	//begin animation cycle
	interceptor.matrixMultiFrameView.CycleFrames(
		[]*ColorFrame{&interceptor.imageScreen, &interceptor.raftStateScreen},
		[]time.Duration{time.Second*2, time.Second*5},
		[]FrameTransition{Slide, Slide})

	interceptor.networksEnabled = make([]bool,len(interceptor.networkDisplays))
		
	interceptor.s1Data = s1Data
	interceptor.s2Data = s2Data
	interceptor.interactiveChans = interactiveChans

	interceptor.HandleInteractive()
		
	return &interceptor
}

func (interceptor *Interceptor) HandleInteractive() {
	if interceptor.s1Data != nil {
		go func() {
			for true {
				enabled := (<- interceptor.s1Data) == 0
				interceptor.networksEnabled[0] = enabled
				interceptor.raftHandlers[0].enabled = enabled
				interceptor.raftHandlers[1].enabled = enabled
			}
		}()
	}
	
	//if interceptor.s2Data != nil  && len(interceptor.networksEnabled) > 1 {
	if interceptor.s2Data != nil {
		go func() {
			for true {
				enabled := (<-interceptor.s2Data == 0)
				interceptor.networksEnabled[1] = enabled
				interceptor.raftHandlers[2].enabled = enabled
				interceptor.raftHandlers[3].enabled = enabled
			}
		}()
	}
	
	if interceptor.interactiveChans != nil {
		//TODO
	}

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
		colors := MakeColorFrame(3 + len(event.Args.Entries), 1, MakeColor(255,255,255))
		colors.Set(len(colors[0])-2, 0, RpcColors["AppendEntries"], Error)
		for i := 0; i < len(colors[0])-3; i++ {
			colors.Set(i,0, event.Args.Entries[i].Command.(SetPixelCommand).PixelColor, Error)
		}

		animation := MakeMovingSegmentAnimation(colors, interceptor.networkDisplays[event.Peer].Width, event.Outgoing)

		go interceptor.networkDisplays[event.Peer].DrawAnimation(animation, calcFps(len(animation)))
	case raft.AppendEntriesResponseEvent:
		colors := MakeColorFrame(4, 1, MakeColor(255,255,255))
		colors.Set(len(colors[0])-2, 0, RpcColors["AppendEntriesResponse"], Error)
		if event.Reply.Success {
			colors.Set(0,0, MakeColor(0,255,0), Error)
		} else {
			colors.Set(0,0, MakeColor(255,0,0), Error)
		}

		animation := MakeMovingSegmentAnimation(colors, interceptor.networkDisplays[event.Peer].Width, event.Outgoing)
		go interceptor.networkDisplays[event.Peer].DrawAnimation(animation, calcFps(len(animation)))

	case raft.RequestVoteEvent:
		colors := MakeColorFrame(4, 1, MakeColor(255,255,255))
		colors.Set(len(colors[0])-2, 0, RpcColors["RequestVote"], Error)
		colors.Set(0,0, interceptor.idColor, Error)

		animation := MakeMovingSegmentAnimation(colors, interceptor.networkDisplays[event.Peer].Width, event.Outgoing)
		go interceptor.networkDisplays[event.Peer].DrawAnimation(animation, calcFps(len(animation)))

	case raft.RequestVoteResponseEvent:
		colors := MakeColorFrame(4, 1, MakeColor(255,255,255))
		colors.Set(len(colors[0])-2, 0, RpcColors["RequestVoteResponse"], Error)
		if event.Reply.VoteGranted {
			colors.Set(0,0, MakeColor(0,255,0), Error)
		} else {
			colors.Set(0,0, MakeColor(255,0,0), Error)
		}

		animation := MakeMovingSegmentAnimation(colors, interceptor.networkDisplays[event.Peer].Width, event.Outgoing)
		go interceptor.networkDisplays[event.Peer].DrawAnimation(animation, calcFps(len(animation)))

	case raft.StartEvent:
		colors := MakeColorFrame(4, 1, MakeColor(255,255,255))
		colors.Set(len(colors[0])-2, 0, RpcColors["StartEvent"], Error)
		if command, ok := event.Args.Command.(SetPixelCommand); ok {
			colors.Set(0,0, command.PixelColor, Error)
		}

		animation := MakeMovingSegmentAnimation(colors, interceptor.networkDisplays[event.Peer].Width, event.Outgoing)
		go interceptor.networkDisplays[event.Peer].DrawAnimation(animation, calcFps(len(animation)))

	case raft.StartResponseEvent:
		colors := MakeColorFrame(4, 1, MakeColor(255,255,255))
		colors.Set(len(colors[0])-2, 0, RpcColors["StartEventResponse"], Error)
		if event.Reply.IsLeader {
			colors.Set(0,0, MakeColor(0,255,0), Error)
		} else {
			colors.Set(0,0, MakeColor(255,0,0), Error)
		}

		animation := MakeMovingSegmentAnimation(colors, interceptor.networkDisplays[event.Peer].Width, event.Outgoing)
		go interceptor.networkDisplays[event.Peer].DrawAnimation(animation, calcFps(len(animation)))

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
func MakeMovingSegmentAnimation(colors ColorFrame, length int, reverseDirection bool) []ColorFrame {
	frameCount := len(colors[0])+ length
	frames := make([]ColorFrame, frameCount)
	for frame := 0; frame < frameCount; frame++ {
		frames[frame] = MakeColorFrame(length, 1, MakeColor(0,0,0))
		beginIndex := frame - (len(colors[0])-1)
		if reverseDirection {
			beginIndex = length-1 - frame
		}
		for i := range colors[0] {
			color := colors[0][i]
			if reverseDirection {
				color = colors[0][len(colors[0])-1-i]
			}
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
	enabled bool
}

func NewRaftHandler(interceptor *Interceptor, listenPort int, forwardAddress string, peer int, outgoing bool) *RaftHandler {

	rh := RaftHandler{}
	rh.interceptor = interceptor
	rh.peer = peer
	rh.outgoing = outgoing
	rh.enabled = false
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
			if rh.enabled {
				reply := raft.StartReply{}
				success := rh.forwardClient.Call("Raft.Start", raft.StartArgs{SetPixelCommand{rand.Intn(8),rand.Intn(8),MakeColorHue(uint32(rand.Int31n(256)))}}, &reply)
				if success {
					fmt.Printf("Start hello: %v\n", reply)
				} else {
					fmt.Printf("err\n")
				}
			}
		}
	}()

	return &rh
}

/*** RAFT RPCs **/
func (rh *RaftHandler) RequestVote(args raft.RequestVoteArgs, reply *raft.RequestVoteReply) error {
	rh.interceptor.OnEventHandler(raft.RequestVoteEvent{args, rh.peer, rh.outgoing})
	if rh.enabled {
		time.Sleep(NetworkForwardDelay)
		success := rh.forwardClient.Call("Raft.RequestVote", args, reply)
		if success {
			rh.interceptor.OnEventHandler(raft.RequestVoteResponseEvent{*reply, rh.peer, !rh.outgoing})
			time.Sleep(NetworkForwardDelay)
		}
	}
	return nil
}

func (rh *RaftHandler) AppendEntries(args raft.AppendEntriesArgs, reply *raft.AppendEntriesReply) error {
	rh.interceptor.OnEventHandler(raft.AppendEntriesEvent{args, rh.peer, rh.outgoing})
	if rh.enabled {
		time.Sleep(NetworkForwardDelay)
		success := rh.forwardClient.Call("Raft.AppendEntries", args, reply)
		if success {
			rh.interceptor.OnEventHandler(raft.AppendEntriesResponseEvent{*reply, rh.peer, !rh.outgoing})
			time.Sleep(NetworkForwardDelay)
		}
	}
	return nil
}

func (rh *RaftHandler) Start(args raft.StartArgs, reply *raft.StartReply) error {
	rh.interceptor.OnEventHandler(raft.StartEvent{args, rh.peer, rh.outgoing})
	if rh.enabled {
		time.Sleep(NetworkForwardDelay)
		success := rh.forwardClient.Call("Raft.Start", args, reply)
		if success {
			rh.interceptor.OnEventHandler(raft.StartResponseEvent{*reply, rh.peer, !rh.outgoing})
			time.Sleep(NetworkForwardDelay)
		}
	}
	return nil
}

func init() {
	gob.Register(SetPixelCommand{})
	gob.Register(MakeColorFrame(0,0,0))
}
