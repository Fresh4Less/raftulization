package raft

import(
	"time"
	"encoding/gob"
)

type RaftEvent interface {
}

type LogUpdatedEvent struct {
	Logs []Log
}

type EntryCommittedEvent struct {
	ApplyMsg ApplyMsg
}

type SetElectionTimeoutEvent struct {
	Duration time.Duration
}

type SetHeartbeatTimeoutEvent struct {
	Duration time.Duration
}

type SetServerStateEvent struct {
	State ServerState
	Term int
}

type AppendEntriesEvent struct {
	Args AppendEntriesArgs
	Peer int
	Outgoing bool
}

type AppendEntriesResponseEvent struct {
	Reply AppendEntriesReply
	Peer int
	Outgoing bool
}

type RequestVoteEvent struct {
	Args RequestVoteArgs
	Peer int
	Outgoing bool
}

type RequestVoteResponseEvent struct {
	Reply RequestVoteReply
	Peer int
	Outgoing bool
}

func init() {
	gob.Register(LogUpdatedEvent{})
	gob.Register(EntryCommittedEvent{})
	gob.Register(SetElectionTimeoutEvent{})
	gob.Register(SetHeartbeatTimeoutEvent{})
	gob.Register(SetServerStateEvent{})
	gob.Register(AppendEntriesEvent{})
	gob.Register(AppendEntriesResponseEvent{})
	gob.Register(RequestVoteEvent{})
	gob.Register(RequestVoteResponseEvent{})
}

