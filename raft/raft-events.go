package raft

import(
	"time"
	"encoding/gob"
)

type RaftEvent interface {
}

//something in raft changed--just send all the data we care about
type StateUpdatedEvent struct {
	State ServerState
	Term int
	LogLength int
	LastApplied int
	RecentLogs []Log //last 8 logs
	VotedFor int
	ReceivedVotes []bool
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

type StartEvent struct {
	Args StartArgs
	Peer int
	Outgoing bool
}

type StartResponseEvent struct {
	Reply StartReply
	Peer int
	Outgoing bool
}

func init() {
	gob.Register(StateUpdatedEvent{})
	gob.Register(EntryCommittedEvent{})
	gob.Register(SetElectionTimeoutEvent{})
	gob.Register(SetHeartbeatTimeoutEvent{})
	gob.Register(AppendEntriesEvent{})
	gob.Register(AppendEntriesResponseEvent{})
	gob.Register(RequestVoteEvent{})
	gob.Register(RequestVoteResponseEvent{})
}

