package main

import(
	"github.com/fresh4less/raftulization/raft"
	"time"
)

type RaftEvent interface {
}

type LogUpdatedEvent struct {
}

type EntryCommittedEvent struct {
	Command interface{}
}

type SetElectionTimeoutEvent struct {
	Duration time.Duration
}

type SetHeartbeatTimeoutEvent struct {
	Duration time.Duration
}

type SetServerStateEvent struct {
	State raft.ServerState
	Term int
}

type AppendEntriesEvent struct {
	Args raft.AppendEntriesArgs
	Peer int
	Outgoing bool
}

type AppendEntriesResponseEvent struct {
	Reply raft.AppendEntriesReply
	Peer int
	Outgoing bool
}

type RequestVoteEvent struct {
	Args raft.RequestVoteArgs
	Peer int
	Outgoing bool
}

type RequestVoteResponseEvent struct {
	Reply raft.RequestVoteReply
	Peer int
	Outgoing bool
}

