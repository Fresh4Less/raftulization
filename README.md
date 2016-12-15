# RAFTULIZATION
## A physical interactive LED visualization for the RAFT consensus algorithm

## Install
`go get github.com/fresh4less/raftulization`

## Build
`go install`

To enable pixel support, use `go install -tags pixelsupport`. Requires ws2811 headers and library to be in system include/lib paths.
To enable interactive support, use `go install -tags rpiosupport`. Go get "github.com/fresh4less/raftulization/switchIO" and "github.com/fresh4less/raftulization/rotaryEncoderIO" 

## Run
Run RAFT:

`./raftulization raft`

Flags:
 - `-s`: Listen port
 - `-c`: Comma separated list of client ip addresses
 - `-e`: Interceptor event ip address
 - `-f`: RAFT state file

Run Interceptor:

`./raftulization intercept`

Flags:
 - `-e`: Event listen port
 - `-s`: RAFT source ip address
 - `-f`: Comma separated list of forwarding addresses. Each address is of the form `P1~P2~C` where P1 is the source->client forwarding port,
         P2 is the client->source forwarding port, and C is the client ip address.

Run LED test:

`./raftulization ledtest`

If pixelsupport is enabled and you get device errors, make sure you run as sudo.
