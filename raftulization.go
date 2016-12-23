package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/fresh4less/raftulization/raft"
	"github.com/fresh4less/neopixel-display/neopixeldisplay"
	"github.com/fresh4less/raftulization/switchIO"
	"github.com/fresh4less/raftulization/rotaryEncoderIO"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

type InteractiveChannels struct {
	buttonBig 	chan int
	buttonL 	chan int
	buttonM		chan int
	buttonR		chan int
	rotaryL		chan int
	rotaryM		chan int
	rotaryR		chan int
}

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
	case "ledtest":
		doLedTest()
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

	raftFlagSet.Parse(os.Args[2:])
	fmt.Printf("Saving state at %v\n", *raftStatePath)
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

	colorState := neopixeldisplay.MakeColorFrame(8,8,neopixeldisplay.MakeColor(0,0,0))

	for true {
		select {
		case applyMsg := <-applyCh:
			go func() {
				if command, ok := applyMsg.Command.(SetPixelCommand); ok {
					colorState.Set(command.X, command.Y, command.PixelColor, neopixeldisplay.Error)
					eventCh <- raft.EntryCommittedEvent{applyMsg, colorState}
				}
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
	displayMode := interceptFlagSet.String("d", "none", "Display mode (neopixel, console, none)")
	pixelBrightness := interceptFlagSet.Float64("b", 1.0, "Pixel brightness (requires -p=true)")
	isInteractive := interceptFlagSet.Bool("i", false, "Set for interactive mode (requires -p=true)")
	s1Pin := interceptFlagSet.Int("s1", -1, "Set the pin for the outer line switch.")
	s2Pin := interceptFlagSet.Int("s2", -1, "Set the pin for the inner line switch.")
	buttonBigPin := interceptFlagSet.Int("buttonBig", -1, "Set the pin for the big button. (requires -i=true)")
	buttonLPin := interceptFlagSet.Int("buttonL", -1, "Set the pin for the left rotary button. (requires -i=true)")
	buttonMPin := interceptFlagSet.Int("buttonM", -1, "Set the pin for the middle rotary button. (requires -i=true)")
	buttonRPin := interceptFlagSet.Int("buttonR", -1, "Set the pin for the right rotary button. (requires -i=true)")
	rotaryL1Pin := interceptFlagSet.Int("rotaryL1", -1, "Set the pin for the left rotary data 1. (requires -i=true)")
	rotaryL2Pin := interceptFlagSet.Int("rotaryL2", -1, "Set the pin for the left rotary data 2. (requires -i=true)")
	rotaryM1Pin := interceptFlagSet.Int("rotaryM1", -1, "Set the pin for the middle rotary data 1. (requires -i=true)")
	rotaryM2Pin := interceptFlagSet.Int("rotaryM2", -1, "Set the pin for the middle rotary data 2. (requires -i=true)")
	rotaryR1Pin := interceptFlagSet.Int("rotaryR1", -1, "Set the pin for the right rotary data 1. (requires -i=true)")
	rotaryR2Pin := interceptFlagSet.Int("rotaryR2", -1, "Set the pin for the right rotary data 2. (requires -i=true)")
	raftId := interceptFlagSet.Int("id", 0, "Set the raft Id")
	
	
	forwardInfo := NetForwardInfoList{}
	interceptFlagSet.Var(&forwardInfo, "f", "comma separated list of forward info in form inPort~outPort~remoteAddress")

	//used to send start
	peerInterceptors := IpAddressList{}
	interceptFlagSet.Var(&peerInterceptors, "o", "comma separated list of peer interceptor network addresses")

	interceptFlagSet.Parse(os.Args[2:])

	var neopixelDisplay neopixeldisplay.PixelDisplay
	
	var s1Data chan int
	var s2Data chan int
	
	if *displayMode == "neopixel" {
		s1Data = switchIO.NewSwitchIO(*s1Pin)
		s2Data = switchIO.NewSwitchIO(*s2Pin)
		if *isInteractive {
			neopixelDisplay = neopixeldisplay.NewNeopixelDisplay(18, 64+30+20+64, 255)
		} else {
			neopixelDisplay = neopixeldisplay.NewNeopixelDisplay(18, 64+30+20, 255)
		}
	} else if *displayMode == "console" {
		if *isInteractive {
			neopixelDisplay = neopixeldisplay.NewConsoleColorDisplay(64+30+20+64, [][]int{[]int{8,8},[]int{30,1},[]int{20,1},[]int{8,8}})
		} else {
			neopixelDisplay = neopixeldisplay.NewConsoleColorDisplay(64+30+20, [][]int{[]int{8,8},[]int{30,1},[]int{20,1}})
		}
	} else {
		neopixelDisplay = neopixeldisplay.NewFakeDisplay(64 + 30 + 20 + 64)
	}

	if len(forwardInfo) < 2 {
		s2Data = nil
	}

	brightness := float32(*pixelBrightness)

	matrixDisplay := neopixeldisplay.NewScreenView(neopixelDisplay, 0, 8, 8, brightness, neopixeldisplay.Error)
	networkDisplays := []*neopixeldisplay.ScreenView{
		neopixeldisplay.NewScreenView(neopixelDisplay, 64, 30, 1, brightness, neopixeldisplay.Error),
		neopixeldisplay.NewScreenView(neopixelDisplay, 64+30, 20, 1, brightness, neopixeldisplay.Error),
	}
	var interactiveDisplay *neopixeldisplay.ScreenView

	var interactiveChans *InteractiveChannels
	
	if *isInteractive {
		interactiveChans = &InteractiveChannels {
			switchIO.NewSwitchIO(*buttonBigPin),
			switchIO.NewSwitchIO(*buttonLPin),
			switchIO.NewSwitchIO(*buttonMPin),
			switchIO.NewSwitchIO(*buttonRPin),
			rotaryEncoderIO.NewRotaryEncoderIO(*rotaryL1Pin,*rotaryL2Pin),
			rotaryEncoderIO.NewRotaryEncoderIO(*rotaryM1Pin,*rotaryM2Pin),
			rotaryEncoderIO.NewRotaryEncoderIO(*rotaryR1Pin,*rotaryR2Pin),
		}
		interactiveDisplay = neopixeldisplay.NewScreenView(neopixelDisplay, 64+30+20, 8,8, brightness, neopixeldisplay.Error)
	}

	NewInterceptor(*eventListenPort, *sourceAddress, forwardInfo, matrixDisplay, networkDisplays, interactiveDisplay, s1Data, s2Data, interactiveChans, peerInterceptors, *raftId)
	
	select{}
}

func doLedTest() {
	ledTestFlagSet := flag.NewFlagSet("", flag.ExitOnError)
	pixelBrightness := ledTestFlagSet.Float64("b", 1.0, "Pixel brightness")
	ledTestFlagSet.Parse(os.Args[2:])

	display := neopixeldisplay.NewNeopixelDisplay(18, 64+30+20, 255)
	matrixScreen := neopixeldisplay.NewScreenView(display, 0, 8, 8, float32(*pixelBrightness), neopixeldisplay.Error)
	matrixTransitionView := neopixeldisplay.NewTransitionView(matrixScreen.GetFrame())
	transition1 := matrixTransitionView.AddTransition(time.Second, neopixeldisplay.Slide)
	transition2 := matrixTransitionView.AddTransition(time.Second, neopixeldisplay.Slide)
	transition3 := matrixTransitionView.AddTransition(time.Second*3, neopixeldisplay.Slide)
	transition1.Frame.SetRect(0, 0, neopixeldisplay.MakeColorFrame(8, 8, neopixeldisplay.MakeColor(255, 0, 0)), neopixeldisplay.Error)
	transition2.Frame.SetRect(0, 0, neopixeldisplay.MakeColorFrame(8, 8, neopixeldisplay.MakeColor(0, 255, 0)), neopixeldisplay.Error)
	transition3.Frame.SetRect(0, 0, neopixeldisplay.MakeColorFrame(8, 8, neopixeldisplay.MakeColor(0, 0, 255)), neopixeldisplay.Error)

	layerView := neopixeldisplay.NewLayerView(transition2.Frame)
	layer1 := layerView.AddLayer(neopixeldisplay.Add)
	layer1.Frame.SetRect(0, 0, neopixeldisplay.MakeColorFrame(7, 7, neopixeldisplay.MakeColor(255, 0, 0)), neopixeldisplay.Error)
	layer2 := layerView.AddLayer(neopixeldisplay.Add)
	layer2.Frame.SetRect(2, 2, neopixeldisplay.MakeColorFrame(5, 5, neopixeldisplay.MakeColor(0, 255, 0)), neopixeldisplay.Error)
	layer3 := layerView.AddLayer(neopixeldisplay.Overwrite)
	layer3.Frame.SetRect(4, 4, neopixeldisplay.MakeColorFrame(3, 3, neopixeldisplay.MakeColor(0, 0, 255)), neopixeldisplay.Error)
	layer4 := layerView.AddLayer(neopixeldisplay.SetWhite)
	layer4.Frame.SetRect(6, 6, neopixeldisplay.MakeColorFrame(2, 2, neopixeldisplay.MakeColor(100, 100, 100)), neopixeldisplay.Error)

	animationView := neopixeldisplay.NewAnimationView(transition3.Frame)
	animationView.PlayAnimation(
		[]neopixeldisplay.ColorFrame{
			neopixeldisplay.MakeColorFrame(8, 8, neopixeldisplay.MakeColorHue(0)),
			neopixeldisplay.MakeColorFrame(8, 8, neopixeldisplay.MakeColorHue(30)),
			neopixeldisplay.MakeColorFrame(8, 8, neopixeldisplay.MakeColorHue(60)),
			neopixeldisplay.MakeColorFrame(8, 8, neopixeldisplay.MakeColorHue(90)),
			neopixeldisplay.MakeColorFrame(8, 8, neopixeldisplay.MakeColorHue(120)),
			neopixeldisplay.MakeColorFrame(8, 8, neopixeldisplay.MakeColorHue(150)),
			neopixeldisplay.MakeColorFrame(8, 8, neopixeldisplay.MakeColorHue(180)),
			neopixeldisplay.MakeColorFrame(8, 8, neopixeldisplay.MakeColorHue(210)),
			neopixeldisplay.MakeColorFrame(8, 8, neopixeldisplay.MakeColorHue(240)),
		},
		10, true)

	stripColors := []neopixeldisplay.Color{
		neopixeldisplay.MakeColor(uint32(*pixelBrightness*float64(255)),0,0),
		neopixeldisplay.MakeColor(0, uint32(*pixelBrightness*float64(255)), 0),
		neopixeldisplay.MakeColor(0, 0, uint32(*pixelBrightness*float64(255))),
	}

	strip1Screen := neopixeldisplay.NewScreenView(display, 64, 30, 1, float32(*pixelBrightness), neopixeldisplay.Error)
	strip2Screen := neopixeldisplay.NewScreenView(display, 64+30, 20, 1, float32(*pixelBrightness), neopixeldisplay.Error)

	t := 0
	for true {
		transition1.Frame.SetRect(0,0, neopixeldisplay.MakeColorNumberChar2x3(t%10, neopixeldisplay.MakeColor(255,0,0), neopixeldisplay.MakeColor(0,0,0)), neopixeldisplay.Error)
		transition1.Frame.Draw()

		//multiAnimView.AddAnimation(MakeMovingSegmentAnimation(MakeColorFrame(5, 1, colors[t%3]), stripDisplay.Width, t%2==0), float32(30/((t%4)+1)))
		//multiAnimView.AddAnimation(MakeMovingSegmentAnimation(MakeColorFrame(5, 1, colors[t%3]), stripDisplay.Width, t%2==0), 60)
		strip1Screen.GetFrame().SetRect(0,0,neopixeldisplay.MakeColorFrame(strip1Screen.Width, strip1Screen.Height, stripColors[t%3]), neopixeldisplay.Error)
		strip1Screen.Draw()
		strip2Screen.GetFrame().SetRect(0,0,neopixeldisplay.MakeColorFrame(strip2Screen.Width, strip2Screen.Height, stripColors[(t+1)%3]), neopixeldisplay.Error)
		strip2Screen.Draw()
		//for i := 64; i < 64+30; i++ {
			//neopixelDisplay.Set(i, colors[(t+1)%3])
		//}
		//for i := 64+30; i < 64+30+20; i++ {
			//neopixelDisplay.Set(i, colors[(t+2)%3])
		//}
		//neopixelDisplay.Show()
		time.Sleep(1*time.Second)
		t++
	}

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

.\raftulization.exe intercept -e 10001 -s 127.0.0.1:8001 -f 9012~9021~127.0.0.1:8002 -d console
.\raftulization.exe raft -s 8001 -e 127.0.0.1:10001 -c 127.0.0.1:9012 -f r1.state

.\raftulization.exe raft -s 8002 -c 127.0.0.1:9021 -f r2.state
3
*/
