package main

import (
	"encoding/gob"
	"errors"
	"fmt"
	"github.com/fresh4less/neopixel-display/neopixeldisplay"
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

const MinElectionTimeout = time.Second * 4
const HeartbeatTimeout = time.Second * 3
const NetworkForwardDelay = time.Millisecond * 1100

//colors
var StateColors = map[raft.ServerState]neopixeldisplay.Color{
	raft.Follower:  neopixeldisplay.MakeColor(255, 0, 0),
	raft.Candidate: neopixeldisplay.MakeColor(0, 255, 0),
	raft.Leader:    neopixeldisplay.MakeColor(0, 0, 255),
}

var RpcColors = map[string]neopixeldisplay.Color {
	"AppendEntries": neopixeldisplay.MakeColor(255,0,0),
	"AppendEntriesResponse": neopixeldisplay.MakeColor(255,165,0),
	"RequestVote": neopixeldisplay.MakeColor(0,0,255),
	"RequestVoteResponse": neopixeldisplay.MakeColor(0,165,255),
	"StartEvent": neopixeldisplay.MakeColor(0,255,0),
	"StartEventResponse": neopixeldisplay.MakeColor(165,255,0),
}

var ElectionTimeoutColor = neopixeldisplay.MakeColor(0,255,0)
var HeartbeatTimeoutColor = neopixeldisplay.MakeColor(255,70,70)

var RaftIdColors = map[int]neopixeldisplay.Color {
	0: neopixeldisplay.MakeColorHue(25),
	1: neopixeldisplay.MakeColorHue(25+50),
	2: neopixeldisplay.MakeColorHue(25+50*2),
	3: neopixeldisplay.MakeColorHue(25+50*3),
	4: neopixeldisplay.MakeColorHue(25+50*4),
}

type NetForwardInfo struct {
	SourceListenPort int

	RemoteListenPort int
	ForwardAddress   string
}

type SetPixelCommand struct {
	X, Y int
	PixelColor neopixeldisplay.Color
}

type Interceptor struct {
	listener        *net.Listener
	rpcServer       *rpc.Server
	raftHandlers    []*RaftHandler

	matrixScreen   *neopixeldisplay.ScreenView
	networkScreens []*neopixeldisplay.ScreenView
	interactiveScreen *neopixeldisplay.ScreenView

	raftStateFrame *neopixeldisplay.ColorFrame
	imageFrame *neopixeldisplay.ColorFrame
	networkLayerViews []*neopixeldisplay.LayerView
	networkAnimLayerViews []*neopixeldisplay.LayerView
	matrixTransitionView *neopixeldisplay.TransitionView
	
	networksEnabled []bool
	
	s1Data	chan int
	s2Data	chan int
	interactiveChans	*InteractiveChannels

	interactivePanel	*InteractivePanel

	timeoutIndex int //used to check if the timeout was cancelled
	timeoutDisplayIndex int
	timeoutColor neopixeldisplay.Color

	demoModeEnabled bool
	peerInterceptorRpcClients []*raft.UnreliableRpcClient
	raftId int
}

func NewInterceptor(eventListenPort int, sourceAddress string, forwardInfo []NetForwardInfo, matrixScreen *neopixeldisplay.ScreenView, networkScreens []*neopixeldisplay.ScreenView, interactiveScreen *neopixeldisplay.ScreenView, s1Data, s2Data chan int, interactiveChans *InteractiveChannels, peerInterceptors IpAddressList, raftId int) *Interceptor {
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

	interceptor.matrixScreen = matrixScreen
	interceptor.networkScreens = networkScreens
	interceptor.interactiveScreen = interactiveScreen

	for _, networkScreen := range interceptor.networkScreens {
		//each network strip has a 2-layer view. the bottom layer is the red bg, top layer is the animation layer
		layerView := neopixeldisplay.NewLayerView(networkScreen.GetFrame())
		layerView.AddLayer(neopixeldisplay.Overwrite)
		animLayer := layerView.AddLayer(neopixeldisplay.Overwrite)
		animLayerView := neopixeldisplay.NewLayerView(animLayer.Frame)
		interceptor.networkLayerViews = append(interceptor.networkLayerViews, layerView)
		interceptor.networkAnimLayerViews = append(interceptor.networkLayerViews, animLayerView)
	}

	interceptor.matrixTransitionView = neopixeldisplay.NewTransitionView(matrixScreen.GetFrame())
	
	//begin animation cycle
	interceptor.raftStateFrame = interceptor.matrixTransitionView.AddTransition(time.Duration(time.Second*5), neopixeldisplay.Slide).Frame
	interceptor.imageFrame = interceptor.matrixTransitionView.AddTransition(time.Duration(time.Second*5), neopixeldisplay.Slide).Frame

	interceptor.networksEnabled = make([]bool,len(interceptor.networkScreens))
	for i := range interceptor.networksEnabled {
		interceptor.networksEnabled[i] = true
	}

	interceptor.s1Data = s1Data
	interceptor.s2Data = s2Data
	interceptor.interactiveChans = interactiveChans

	if(interactiveChans != nil) {
		interceptor.interactivePanel = NewInteractivePanel(8,8)
		interceptor.interactiveScreen.GetFrame().SetRect(0,0,interceptor.interactivePanel.GetColorFrame(), neopixeldisplay.Error)
		interceptor.interactiveScreen.Draw()
	}

	for _, ip := range peerInterceptors {
		interceptor.peerInterceptorRpcClients = append(interceptor.peerInterceptorRpcClients, raft.NewUnreliableRpcClient(ip, 5, time.Second))
	}

	interceptor.raftId = raftId
	
	interceptor.HandleInteractive()
	interceptor.RunDemoMode()
		
	return &interceptor
}

func (interceptor *Interceptor) RunDemoMode() {
	go func() {
		for true {
			time.Sleep(time.Duration(time.Duration(5+rand.Intn(5))*time.Second))
			if interceptor.demoModeEnabled {
				interceptor.SendStart(SetPixelCommand{rand.Intn(8),rand.Intn(8),neopixeldisplay.MakeColorHue(uint32(rand.Int31n(256)))})
			}
		}
	}()
}

func (interceptor *Interceptor) HandleInteractive() {
	if interceptor.s1Data != nil {
		go func() {
			for true {
				enabled := (<-interceptor.s1Data) == 0
				interceptor.networksEnabled[0] = enabled
				interceptor.raftHandlers[0].enabled = enabled
				interceptor.raftHandlers[1].enabled = enabled
				
				if enabled {
					interceptor.networkLayerViews[0].GetLayer(0).Frame.SetRect(0,0,neopixeldisplay.MakeColorFrame(30,1,neopixeldisplay.MakeColor(0,0,0)), neopixeldisplay.Error)
				} else {
					interceptor.networkLayerViews[0].GetLayer(0).Frame.SetRect(0,0,neopixeldisplay.MakeColorFrame(30,1,neopixeldisplay.MakeColor(0,0,10)), neopixeldisplay.Error)
				}
				interceptor.networkLayerViews[0].Draw()
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
				if enabled {
					interceptor.networkLayerViews[1].GetLayer(0).Frame.SetRect(0,0,neopixeldisplay.MakeColorFrame(20,1,neopixeldisplay.MakeColor(0,0,0)), neopixeldisplay.Error)
				} else {
					interceptor.networkLayerViews[1].GetLayer(0).Frame.SetRect(0,0,neopixeldisplay.MakeColorFrame(20,1,neopixeldisplay.MakeColor(0,0,10)), neopixeldisplay.Error)
				}
				interceptor.networkLayerViews[1].Draw()
			}
		}()
	}
	
	if interceptor.interactiveChans != nil {
	
		go func() {
			for true {
				turnedLeft := (<-interceptor.interactiveChans.rotaryL == 0)
				if turnedLeft {
					interceptor.interactivePanel.MoveLeft()
				} else {
					interceptor.interactivePanel.MoveRight()
				}
				interceptor.interactiveScreen.GetFrame().SetRect(0,0,interceptor.interactivePanel.GetColorFrame(), neopixeldisplay.Error)
				interceptor.interactiveScreen.Draw()
			}
		}()
		go func() {
			for true {
				turnedLeft := (<-interceptor.interactiveChans.rotaryM == 0)
				if turnedLeft {
					interceptor.interactivePanel.MoveDown()
				} else {
					interceptor.interactivePanel.MoveUp()
				}
				interceptor.interactiveScreen.GetFrame().SetRect(0,0,interceptor.interactivePanel.GetColorFrame(), neopixeldisplay.Error)
				interceptor.interactiveScreen.Draw()
			}
		}()
		
		go func() {
			for true {
				turnedLeft := (<-interceptor.interactiveChans.rotaryR == 0)
				if turnedLeft {
					interceptor.interactivePanel.DecrementHue()
				} else {
					interceptor.interactivePanel.IncrementHue()
				}
				interceptor.interactiveScreen.GetFrame().SetRect(0,0,interceptor.interactivePanel.GetColorFrame(), neopixeldisplay.Error)
				interceptor.interactiveScreen.Draw()
			}
		}()
	
		go func() {
			for true {
				bigButtonPressed := (<-interceptor.interactiveChans.buttonBig == 0)
				if bigButtonPressed {
					interceptor.SendStart(SetPixelCommand{
						interceptor.interactivePanel.x,
						interceptor.interactivePanel.y,
						neopixeldisplay.MakeColorHue(interceptor.interactivePanel.hue),
					})
				}
			}
		}()
		//TODO: do something with pressing in the 3 rotary encoders.
		go func() {
			for true {
				toggleModeButonPressed := (<-interceptor.interactiveChans.buttonR == 0)
				if toggleModeButonPressed {
					interceptor.demoModeEnabled = !interceptor.demoModeEnabled
				}
			}
		}()
	}

}

func (interceptor *Interceptor) BeginTimeoutAnimation(duration time.Duration, maxDuration time.Duration, color neopixeldisplay.Color) {
	interceptor.timeoutIndex++
	timeoutIndex := interceptor.timeoutIndex
	interceptor.timeoutColor = color
	go func() {
		beginIndex := int(float32(duration)/float32(maxDuration) * float32(8))
		for i := beginIndex; i >= 0; i-- {
			interceptor.timeoutDisplayIndex = i
			interceptor.raftStateFrame.SetRect(0, 3, neopixeldisplay.MakeColorFrame(8,1, neopixeldisplay.MakeColor(0,0,0)), neopixeldisplay.Error)
			interceptor.raftStateFrame.SetRect(0, 3, neopixeldisplay.MakeColorFrame(interceptor.timeoutDisplayIndex, 1, interceptor.timeoutColor), neopixeldisplay.Error)
			interceptor.raftStateFrame.Draw()
			time.Sleep(maxDuration/9)
			if timeoutIndex != interceptor.timeoutIndex {
				return
			}
		}
	}()
}

func (interceptor *Interceptor) SendStart(command SetPixelCommand) {
	go func() {
		reply := raft.StartReply{}
		interceptor.raftHandlers[1].GetForwardClient().Call("Raft.Start", raft.StartArgs{command}, &reply)
	}()
	for i, handler := range interceptor.raftHandlers {
		if i%2 == 0 {
			go func(h *RaftHandler) {
				reply := raft.StartReply{}
				h.GetForwardClient().Call("Raft.Start", raft.StartArgs{command}, &reply)
			}(handler)
		}
	}
	for _, peerInterceptor := range interceptor.peerInterceptorRpcClients {
		go func(c *raft.UnreliableRpcClient) {
			reply := raft.StartReply{}
			c.Call("Raft.Start", raft.StartArgs{command}, &reply)
		}(peerInterceptor)
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
		interceptor.BeginTimeoutAnimation(event.Duration, MinElectionTimeout*2, ElectionTimeoutColor)
	case raft.SetHeartbeatTimeoutEvent:
		interceptor.BeginTimeoutAnimation(event.Duration, event.Duration, HeartbeatTimeoutColor)
	case raft.AppendEntriesEvent:
		colors := neopixeldisplay.MakeColorFrame(3 + len(event.Args.Entries), 1, neopixeldisplay.MakeColor(255,255,255))
		colors.Set(colors.Width-2, 0, RpcColors["AppendEntries"], neopixeldisplay.Error)
		for i := 0; i < colors.Width-3; i++ {
			colors.Set(i,0, event.Args.Entries[i].Command.(SetPixelCommand).PixelColor, neopixeldisplay.Error)
		}

		animation := MakeMovingSegmentAnimation(colors, interceptor.networkScreens[event.Peer].Width, event.Outgoing)
		go func() {
			layer := interceptor.networkAnimLayerViews[event.Peer].AddLayer(neopixeldisplay.Add)
			animView := neopixeldisplay.NewAnimationView(layer.Frame)
			done := animView.PlayAnimation(animation, calcFps(len(animation)), false)
			<-done
			interceptor.networkAnimLayerViews[event.Peer].DeleteLayer(layer)
		}()

	case raft.AppendEntriesResponseEvent:
		colors := neopixeldisplay.MakeColorFrame(4, 1, neopixeldisplay.MakeColor(255,255,255))
		colors.Set(colors.Width-2, 0, RpcColors["AppendEntriesResponse"], neopixeldisplay.Error)
		if event.Reply.Success {
			colors.Set(0,0, neopixeldisplay.MakeColor(0,255,0), neopixeldisplay.Error)
		} else {
			colors.Set(0,0, neopixeldisplay.MakeColor(255,0,0), neopixeldisplay.Error)
		}

		animation := MakeMovingSegmentAnimation(colors, interceptor.networkScreens[event.Peer].Width, event.Outgoing)
		go func() {
			layer := interceptor.networkAnimLayerViews[event.Peer].AddLayer(neopixeldisplay.Add)
			animView := neopixeldisplay.NewAnimationView(layer.Frame)
			done := animView.PlayAnimation(animation, calcFps(len(animation)), false)
			<-done
			interceptor.networkAnimLayerViews[event.Peer].DeleteLayer(layer)
		}()

	case raft.RequestVoteEvent:
		colors := neopixeldisplay.MakeColorFrame(4, 1, neopixeldisplay.MakeColor(255,255,255))
		colors.Set(colors.Width-2, 0, RpcColors["RequestVote"], neopixeldisplay.Error)
		colors.Set(0,0, RaftIdColors[interceptor.raftId], neopixeldisplay.Error)

		animation := MakeMovingSegmentAnimation(colors, interceptor.networkScreens[event.Peer].Width, event.Outgoing)
		go func() {
			layer := interceptor.networkAnimLayerViews[event.Peer].AddLayer(neopixeldisplay.Add)
			animView := neopixeldisplay.NewAnimationView(layer.Frame)
			done := animView.PlayAnimation(animation, calcFps(len(animation)), false)
			<-done
			interceptor.networkAnimLayerViews[event.Peer].DeleteLayer(layer)
		}()

	case raft.RequestVoteResponseEvent:
		colors := neopixeldisplay.MakeColorFrame(4, 1, neopixeldisplay.MakeColor(255,255,255))
		colors.Set(colors.Width-2, 0, RpcColors["RequestVoteResponse"], neopixeldisplay.Error)
		if event.Reply.VoteGranted {
			colors.Set(0,0, neopixeldisplay.MakeColor(0,255,0), neopixeldisplay.Error)
		} else {
			colors.Set(0,0, neopixeldisplay.MakeColor(255,0,0), neopixeldisplay.Error)
		}

		animation := MakeMovingSegmentAnimation(colors, interceptor.networkScreens[event.Peer].Width, event.Outgoing)
		go func() {
			layer := interceptor.networkAnimLayerViews[event.Peer].AddLayer(neopixeldisplay.Add)
			animView := neopixeldisplay.NewAnimationView(layer.Frame)
			done := animView.PlayAnimation(animation, calcFps(len(animation)), false)
			<-done
			interceptor.networkAnimLayerViews[event.Peer].DeleteLayer(layer)
		}()

	case raft.StartEvent:
		colors := neopixeldisplay.MakeColorFrame(4, 1, neopixeldisplay.MakeColor(255,255,255))
		colors.Set(colors.Width-2, 0, RpcColors["StartEvent"], neopixeldisplay.Error)
		if command, ok := event.Args.Command.(SetPixelCommand); ok {
			colors.Set(0,0, command.PixelColor, neopixeldisplay.Error)
		}

		animation := MakeMovingSegmentAnimation(colors, interceptor.networkScreens[event.Peer].Width, event.Outgoing)
		go func() {
			layer := interceptor.networkAnimLayerViews[event.Peer].AddLayer(neopixeldisplay.Add)
			animView := neopixeldisplay.NewAnimationView(layer.Frame)
			done := animView.PlayAnimation(animation, calcFps(len(animation)), false)
			<-done
			interceptor.networkAnimLayerViews[event.Peer].DeleteLayer(layer)
		}()

	case raft.StartResponseEvent:
		colors := neopixeldisplay.MakeColorFrame(4, 1, neopixeldisplay.MakeColor(255,255,255))
		colors.Set(colors.Width-2, 0, RpcColors["StartEventResponse"], neopixeldisplay.Error)
		if event.Reply.IsLeader {
			colors.Set(0,0, neopixeldisplay.MakeColor(0,255,0), neopixeldisplay.Error)
		} else {
			colors.Set(0,0, neopixeldisplay.MakeColor(255,0,0), neopixeldisplay.Error)
		}

		animation := MakeMovingSegmentAnimation(colors, interceptor.networkScreens[event.Peer].Width, event.Outgoing)
		go func() {
			layer := interceptor.networkAnimLayerViews[event.Peer].AddLayer(neopixeldisplay.Add)
			animView := neopixeldisplay.NewAnimationView(layer.Frame)
			done := animView.PlayAnimation(animation, calcFps(len(animation)), false)
			<-done
			interceptor.networkAnimLayerViews[event.Peer].DeleteLayer(layer)
		}()

	default:
		fmt.Printf("Unexpected type %T\n", event)
	}
	return true
}

func calcFps(frameCount int) float32 {
	return float32(1000*frameCount)/float32(NetworkForwardDelay/time.Millisecond)
}

func (interceptor *Interceptor) onStateUpdated(event raft.StateUpdatedEvent) {
	interceptor.raftStateFrame.SetAll(neopixeldisplay.MakeColor(0,0,0))
	//id TODO don't hardcode this
	interceptor.raftStateFrame.SetRect(0, 0, neopixeldisplay.MakeColorFrame(2, 2, RaftIdColors[interceptor.raftId]), neopixeldisplay.Error)
	// voted for TODO
	//VotedFor int
	// received votes TODO: use id colors instead of just counting
	for i, voted := range event.ReceivedVotes {
		if voted {
			interceptor.raftStateFrame.Set(2+(i%4), 1, neopixeldisplay.MakeColor(255, 200, 0), neopixeldisplay.Error)
		}
	}
	// state
	interceptor.raftStateFrame.SetRect(6, 0, neopixeldisplay.MakeColorFrame(2, 2, StateColors[event.State]), neopixeldisplay.Error)
	// logs
	for i, log := range event.RecentLogs {
		if setPixelCommand, ok := log.Command.(SetPixelCommand); ok {
			color := setPixelCommand.PixelColor
			if event.LogLength - len(event.RecentLogs) + i >= event.LastApplied {
				//uncommitted, show at half brightness
				color = neopixeldisplay.MakeColor(color.GetRed()/2, color.GetGreen()/2, color.GetBlue()/2)
			}
			interceptor.raftStateFrame.SetRect(i+8-len(event.RecentLogs), 2, neopixeldisplay.MakeColorFrame(1, 1, color), neopixeldisplay.Error)
		}
	}
	
	interceptor.raftStateFrame.SetRect(0, 3, neopixeldisplay.MakeColorFrame(8,1, neopixeldisplay.MakeColor(0,0,0)), neopixeldisplay.Error)
	interceptor.raftStateFrame.SetRect(0, 3, neopixeldisplay.MakeColorFrame(interceptor.timeoutDisplayIndex, 1, interceptor.timeoutColor), neopixeldisplay.Error)

	// term
	interceptor.raftStateFrame.SetRect(0, 5, neopixeldisplay.MakeColorNumberChar2x3(nthDigit(event.Term, 2), neopixeldisplay.MakeColor(255, 255, 255), neopixeldisplay.MakeColor(0, 0, 0)), neopixeldisplay.Error)
	interceptor.raftStateFrame.SetRect(3, 5, neopixeldisplay.MakeColorNumberChar2x3(nthDigit(event.Term, 1), neopixeldisplay.MakeColor(255, 255, 255), neopixeldisplay.MakeColor(0, 0, 0)), neopixeldisplay.Error)
	interceptor.raftStateFrame.SetRect(6, 5, neopixeldisplay.MakeColorNumberChar2x3(nthDigit(event.Term, 0), neopixeldisplay.MakeColor(255, 255, 255), neopixeldisplay.MakeColor(0, 0, 0)), neopixeldisplay.Error)
	interceptor.raftStateFrame.Draw()
}

func (interceptor *Interceptor) onEntryCommitted(event raft.EntryCommittedEvent) {
	interceptor.imageFrame.SetRect(0,0, event.State.(neopixeldisplay.ColorFrame), neopixeldisplay.Error)
	interceptor.imageFrame.Draw()
	//interceptor.matrixMultiFrameView.CycleFrames(
		//[]*ColorFrame{&interceptor.imageScreen, &interceptor.raftStateScreen},
		//[]time.Duration{time.Second*5, time.Second*2},
		//[]FrameTransition{Slide, Slide})
}

// moves horizontally only
func MakeMovingSegmentAnimation(colors neopixeldisplay.ColorFrame, length int, reverseDirection bool) []neopixeldisplay.ColorFrame {
	frameCount := colors.Width + length
	frames := make([]neopixeldisplay.ColorFrame, frameCount)
	for frame := 0; frame < frameCount; frame++ {
		frames[frame] = neopixeldisplay.MakeColorFrame(length, 1, neopixeldisplay.MakeColor(0,0,0))
		beginIndex := frame - (colors.Width-1)
		if reverseDirection {
			beginIndex = length-1 - frame
		}
		for i := 0; i < colors.Width; i++ {
			color := colors.Get(i, 0, neopixeldisplay.Error)
			if reverseDirection {
				color = colors.Get(colors.Width-1-i, 0, neopixeldisplay.Error)
			}
			frames[frame].Set(beginIndex+i,0, color, neopixeldisplay.Clip)
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
	rh.enabled = true
	rh.rpcServer = rpc.NewServer()
	rh.rpcServer.RegisterName("Raft", &rh)

	httpServer := http.NewServeMux()
	httpServer.Handle("/_goRPC_", rh.rpcServer)

	go http.ListenAndServe(":"+strconv.Itoa(listenPort), httpServer)

	fmt.Printf("RaftHandler: Listening on %v\n", listenPort)

	rh.forwardClient = raft.NewUnreliableRpcClient(forwardAddress, 5, time.Second)

	return &rh
}

func (rh *RaftHandler) GetForwardClient() *raft.UnreliableRpcClient {
	return rh.forwardClient
}

/*** RAFT RPCs **/
func (rh *RaftHandler) RequestVote(args raft.RequestVoteArgs, reply *raft.RequestVoteReply) error {
	rh.interceptor.OnEventHandler(raft.RequestVoteEvent{args, rh.peer, rh.outgoing})
	time.Sleep(NetworkForwardDelay)
	if !rh.enabled {
		return errors.New("RPC Dropped")
	}
	success := rh.forwardClient.Call("Raft.RequestVote", args, reply)
	if success {
		rh.interceptor.OnEventHandler(raft.RequestVoteResponseEvent{*reply, rh.peer, !rh.outgoing})
		time.Sleep(NetworkForwardDelay)
	}
	return nil
}

func (rh *RaftHandler) AppendEntries(args raft.AppendEntriesArgs, reply *raft.AppendEntriesReply) error {
	rh.interceptor.OnEventHandler(raft.AppendEntriesEvent{args, rh.peer, rh.outgoing})
	time.Sleep(NetworkForwardDelay)
	if !rh.enabled {
		return errors.New("RPC Dropped")
	}
	success := rh.forwardClient.Call("Raft.AppendEntries", args, reply)
	if success {
		rh.interceptor.OnEventHandler(raft.AppendEntriesResponseEvent{*reply, rh.peer, !rh.outgoing})
		time.Sleep(NetworkForwardDelay)
	}
	return nil
}

func (rh *RaftHandler) Start(args raft.StartArgs, reply *raft.StartReply) error {
	rh.interceptor.OnEventHandler(raft.StartEvent{args, rh.peer, rh.outgoing})
	time.Sleep(NetworkForwardDelay)
	if !rh.enabled {
		return errors.New("RPC Dropped")
	}
	success := rh.forwardClient.Call("Raft.Start", args, reply)
	if success {
		rh.interceptor.OnEventHandler(raft.StartResponseEvent{*reply, rh.peer, !rh.outgoing})
		time.Sleep(NetworkForwardDelay)
	}
	return nil
}

func init() {
	gob.Register(SetPixelCommand{})
	gob.Register(neopixeldisplay.MakeColorFrame(0,0,0))
}
