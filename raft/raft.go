package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	"sync"
	"time"
	"math/rand"
	"fmt"
	"sort"
	"bytes"
	"encoding/gob"
	"io/ioutil"
)

//
// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make().
//
type ApplyMsg struct {
	Index       int
	Command     interface{}
	UseSnapshot bool   // ignore for lab2; only used in lab3
	Snapshot    []byte // ignore for lab2; only used in lab3
}

//
// A Go object implementing a single Raft peer.
//
type Raft struct {
	mu        sync.Mutex
	peers     []*UnreliableRpcClient
	stateFilePath string
	me        int // index into peers[]

	// Your data here.
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.
	applyCh chan ApplyMsg
	eventCh chan RaftEvent

	// persistent (must be saved to disk before rpc response)
	currentTerm int
	votedFor int
	log []Log

	// RAFT variables
	// volatile
	commitIndex int
	lastApplied int

	// volatile leader
	nextIndex []int
	matchIndex []int

	// volatile non-RAFT
	rng *rand.Rand
	serverState ServerState
	electionTimer *time.Timer
	heartbeatTimer *time.Timer
	receivedVote []bool //indexed by peer

	requestVoteRequests chan RequestVoteRequestData
	appendEntriesRequests chan AppendEntriesRequestData

	requestVoteResponses chan RequestVoteResponse
	appendEntriesResponses chan AppendEntriesResponse

	startRequests chan StartRequestData
	Verbosity int
}

type StartRequestData struct {
	command interface{}
	response chan StartResponse
}

type StartResponse struct {
	index int
	term int
	isLeader bool
}

type ServerState int

const (
	Follower ServerState = iota
	Candidate
	Leader
)

type RaftPersistentData struct {
	CurrentTerm int
	VotedFor int
	Log []Log
}

type Log struct {
	Term int
	Command interface{}
}

const MinElectionTimeout = time.Second * 4
const HeartbeatTimeout = time.Second * 3

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {
	return rf.currentTerm, (rf.serverState == Leader)
}

//
// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
//
func (rf *Raft) persist() {
	buffer := bytes.Buffer{}
	encoder := gob.NewEncoder(&buffer)
	// we could just store this struct in the Raft instance, but for my convenience I just copy the values here
	// we could also just encode individual values instead of a struct, but this is better design
	err := encoder.Encode(RaftPersistentData{rf.currentTerm, rf.votedFor, rf.log})
	if err != nil {
		panic(fmt.Sprintf("Raft.persist: Encoding failed: %v", err))
	}

	err = ioutil.WriteFile(rf.stateFilePath, buffer.Bytes(), 0644)
	if err != nil {
		fmt.Printf("Error: failed to write RAFT state to '%s'\n", rf.stateFilePath)
	}
}

//
// restore previously persisted state.
//
func (rf *Raft) restoreState(path string) {
	var persistentData RaftPersistentData

	data, err := ioutil.ReadFile(path)
	if err != nil {
		fmt.Printf("Warning: failed to open RAFT state at '%v'\n")
	}

	buffer := bytes.NewBuffer(data)
	decoder := gob.NewDecoder(buffer)
	err = decoder.Decode(&persistentData)
	if err != nil {
		// use defaults
		fmt.Printf("Using default RAFT state\n")
		persistentData.CurrentTerm = 0
		persistentData.VotedFor = -1
		persistentData.Log = make([]Log, 0)
	}

	rf.currentTerm = persistentData.CurrentTerm
	rf.votedFor = persistentData.VotedFor
	rf.log = persistentData.Log
}

type RequestVoteArgs struct {
	Term int
	CandidateId int
	LastLogIndex int
	LastLogTerm int
}

type RequestVoteReply struct {
	Term int
	VoteGranted bool
}

type RequestVoteRequestData struct {
	args *RequestVoteArgs
	reply *RequestVoteReply
	done chan bool
}

type RequestVoteResponse struct {
	server int
	args RequestVoteArgs
	success bool
	reply *RequestVoteReply
}

func (rf *Raft) RequestVote(args RequestVoteArgs, reply *RequestVoteReply) error {
	if rf.Verbosity >= 4 {
		fmt.Printf("%v: RequestVote from %v (term: %v)\n", rf.me, args.CandidateId, args.Term)
	}
	done := make(chan bool)
	rf.requestVoteRequests <- RequestVoteRequestData{&args, reply, done}
	<-done
	return nil
}

func (rf *Raft) handleRequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	if rf.Verbosity >= 3 {
		fmt.Printf("%v: Handling RequestVote from %v (term: %v)\n", rf.me, args.CandidateId, args.Term)
	}

	defer rf.onStateUpdated()

	if args.Term > rf.currentTerm  {
		rf.becomeFollower(args.Term, false)
	}

	reply.Term = rf.currentTerm
	reply.VoteGranted = false

	if args.LastLogTerm < rf.lastLogTerm() ||
		(args.LastLogTerm == rf.lastLogTerm() && args.LastLogIndex < rf.lastLogIndex()) {
			// candidate doesn't have the "most complete" log
			return
	}

	if args.Term >= rf.currentTerm && (rf.votedFor == -1 || rf.votedFor == args.CandidateId) {
		rf.votedFor = args.CandidateId
		rf.persist()
		reply.Term = rf.currentTerm
		reply.VoteGranted = true
	}
}

// return value is true if a vote was granted
func (rf *Raft) handleRequestVoteResponse(response *RequestVoteResponse) bool {
	if rf.Verbosity >= 3 {
		fmt.Printf("%v: Handling RequestVote response from %v (success: %v, granted: %v, term: %v)\n",
			rf.me, response.server, response.success, response.reply.VoteGranted, response.reply.Term)
	}
	//server, success, reply
	if response.success {
		if response.reply.Term > rf.currentTerm {
			rf.becomeFollower(response.reply.Term, false)
			return false
		}

		if rf.serverState == Candidate {
			if response.reply.Term == rf.currentTerm && response.reply.VoteGranted {
				return true
			}
		}
	} else {
		// resend if we're still in the election
		if response.args.Term == rf.currentTerm && rf.serverState == Candidate {
			go rf.sendRequestVote(response.server, response.args)
		}
	}

	return false
}

func (rf *Raft) sendRequestVote(server int, args RequestVoteArgs) {
	var reply RequestVoteReply
	success := rf.peers[server].Call("Raft.RequestVote", args, &reply)
	if rf.Verbosity >= 4 {
		fmt.Printf("%v: RequestVote response from %v (success: %v, granted: %v, term: %v)\n", rf.me, server, success, reply.VoteGranted, reply.Term)
		}
	rf.requestVoteResponses <- RequestVoteResponse{server, args, success, &reply}
}

type AppendEntriesArgs struct {
	Term int
	LeaderId int
	PrevLogIndex int
	PrevLogTerm int
	Entries []Log
	LeaderCommit int
}

type AppendEntriesReply struct {
	Term int
	Success bool
	ConflictTerm int
	ConflictTermFirstIndex int
}

type AppendEntriesRequestData struct {
	args *AppendEntriesArgs
	reply *AppendEntriesReply
	done chan bool
}

type AppendEntriesResponse struct {
	server int
	lastLogIndex int
	success bool
	reply *AppendEntriesReply
}

func (rf *Raft) AppendEntries(args AppendEntriesArgs, reply *AppendEntriesReply) error {
	if rf.Verbosity >= 4 {
		fmt.Printf("%v: AppendEntries from %v (term: %v)\n", rf.me, args.LeaderId, args.Term)
	}
	done := make(chan bool)
	rf.appendEntriesRequests <- AppendEntriesRequestData{&args, reply, done}
	<-done
	return nil
}

func (rf *Raft) handleAppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	if rf.Verbosity >= 3 {
		fmt.Printf("%v: Handling AppendEntries from %v (term: %v, prevLogIndex: %v, prevLogTerm: %v, entries: %v, leaderCommit: %v)\n",
			rf.me, args.LeaderId, args.Term, args.PrevLogIndex, args.PrevLogTerm, len(args.Entries), args.LeaderCommit)
	}

	reply.ConflictTerm = -1
	reply.ConflictTermFirstIndex = 0

	if args.Term > rf.currentTerm {
		rf.becomeFollower(args.Term, false)
	} else if args.Term == rf.currentTerm {
		// become a follower but don't reset our vote
		rf.becomeFollower(args.Term, true)
	} else {
		// outdated, ignore
		reply.Term = rf.currentTerm
		reply.Success = false
		return
	}

	defer rf.onStateUpdated()

	// process the RPC
	reply.Term = rf.currentTerm

	// assert we are not the Leader
	if rf.serverState == Leader {
		fmt.Printf("%v: ASSERTION FAIL: handleAppendEntries: should not process the RPC as the leader\n", rf.me)
		reply.Success = false
		return
	}

	prevLogTerm := -1
	if args.PrevLogIndex > 0 && args.PrevLogIndex < len(rf.log) {
		prevLogTerm = rf.log[args.PrevLogIndex].Term
	}

	// don't need to check if rf.lastLogIndex < prevLogTerm explicitly
	// --in that case prevLogTerm = -1, which only matches args.PrevLogTerm if we are at the beginning of the log
	if args.PrevLogTerm != prevLogTerm {
		// we are missing logs
		reply.Success = false
		reply.ConflictTerm = prevLogTerm
		if args.PrevLogIndex >= len(rf.log) {
			// our log is too short, return the end of the log
			reply.ConflictTermFirstIndex = len(rf.log)
		} else {
			// find the first index with prevLogTerm
			for i := args.PrevLogIndex; i >= 0 && rf.log[i].Term == prevLogTerm; i-- {
				reply.ConflictTermFirstIndex = i
			}
		}

	} else {
		if len(args.Entries) > 0 {
			// trim extraneous logs and append entries
			rf.log = append(rf.log[0:args.PrevLogIndex+1], args.Entries...)
			rf.persist()
		}

		// update commitIndex
		if args.LeaderCommit > rf.commitIndex {
			rf.commitIndex = minInt(args.LeaderCommit, rf.lastLogIndex())
			rf.applyLogs()
		}

		reply.Success = true
	}
}

func (rf *Raft) handleAppendEntriesResponse(response *AppendEntriesResponse) {
	if rf.Verbosity >= 3 {
		fmt.Printf("%v: Handling AppendEntries response from %v (success: %v, appendSuccess: %v, term: %v)\n",
			rf.me, response.server, response.success, response.reply.Success, response.reply.Term)
	}

	defer rf.onStateUpdated()
	if response.success {
		if response.reply.Success {
			// update values for that server
			rf.nextIndex[response.server] = response.lastLogIndex + 1
			rf.matchIndex[response.server] = response.lastLogIndex

			// check if new logs have been committed

			// this map counts the number of servers with a given matchIndex
			matchIndexMap := make(map[int]int)
			for _, matchIndex := range rf.matchIndex {
				matchIndexMap[matchIndex]++
			}

			// put the keys into an array, in descending order
			keys := make([]int, len(matchIndexMap))
			i := 0
			for k := range matchIndexMap {
				keys[i] = k
				i++
			}
			sort.Sort(sort.Reverse(sort.IntSlice(keys)))

			//walk down the indexes until we have passed the majority of matchIndexes
			minMatch := -1
			counted := 0
			for _, matchIndex := range keys {
				counted += matchIndexMap[matchIndex]
				if counted > len(rf.peers)/2 {
					minMatch = matchIndex
					break
				}
			}

			if minMatch > rf.commitIndex && rf.log[minMatch].Term == rf.currentTerm {
				rf.commitIndex = minMatch
				rf.applyLogs()
			}
		} else {
			if response.reply.Term > rf.currentTerm {
				rf.becomeFollower(response.reply.Term, false)
			} else {
				rf.nextIndex[response.server] = maxInt(rf.nextIndex[response.server] - 1, 0)
				// instead of above line, use the backtracking optimization below
				// NOTE: this has a bug that only occurs in some of the harder tests (figure 8 unreliable and churn), and only about 20% of the time for a given "go test" execution
				// on the other hand, the above code works 100% of the time, but always fails the harder tests due to the tests timing out
				//if response.reply.ConflictTerm == -1 {
					//// move nextIndex to the end of their log
					//rf.nextIndex[response.server] = response.reply.ConflictTermFirstIndex
				//} else {
					//// search back until we find the term, or get to a term with a lower value

					//// in case we don't find anything, set to the ConflictTermFirstIndex
					//rf.nextIndex[response.server] = response.reply.ConflictTermFirstIndex
					//for i := len(rf.log)-1; i >= 0; i-- {
						////fmt.Printf("index %v, len %v\n", i, len(rf.log))
						//if rf.log[i].Term <= response.reply.ConflictTerm {
							//if rf.log[i].Term == response.reply.ConflictTerm {
								//rf.nextIndex[response.server] = i + 1
							//}
							//break
						//}
					//}
				//}
			}
		}
	}
}

func (rf *Raft) sendAppendEntries(server int, args AppendEntriesArgs, lastLogIndex int) {
	var reply AppendEntriesReply
	success := rf.peers[server].Call("Raft.AppendEntries", args, &reply)

	rf.appendEntriesResponses <- AppendEntriesResponse{server, lastLogIndex, success, &reply}
	if rf.Verbosity >= 4 {
		fmt.Printf("%v: AppendEntries response from %v (success: %v, term: %v)\n", rf.me, server, success, reply.Term)
	}
}

func minInt(x, y int) int {
	if x < y {
		return x
	} else {
		return y
	}
}

func maxInt(x, y int) int {
	if x > y {
		return x
	} else {
		return y
	}
}

//
// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
//

type StartArgs struct {
	Command interface{}
}

type StartReply struct {
	Index int
	Term int
	IsLeader bool
}

func (rf *Raft) Start(args *StartArgs, reply *StartReply) error {
	if rf.Verbosity >= 1 {
		fmt.Printf("%v: Command submitted: %v\n", rf.me, args.Command)
	}
	done := make(chan StartResponse)
	rf.startRequests <- StartRequestData{args.Command, done}
	response := <-done
	reply.Index = response.index
	reply.Term = response.term
	reply.IsLeader = response.isLeader
	return nil
}

//
// the tester calls Kill() when a Raft instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
//
func (rf *Raft) Kill() {
	// Your code here, if desired.
}

//
// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
//
func MakeRaft(peerAddresses []string, me int,
	stateFilePath string, applyCh chan ApplyMsg, verbosity int, eventCh chan RaftEvent) *Raft {
	rf := &Raft{}
	rf.peers = make([]*UnreliableRpcClient, len(peerAddresses))
	for index, address := range peerAddresses {
		if index != me {
			rf.peers[index] = NewUnreliableRpcClient(address, 5, time.Second)
		}
	}
	rf.me = me
	rf.applyCh = applyCh
	rf.eventCh = eventCh
	rf.stateFilePath = stateFilePath

	// volatile RAFT variables
	rf.commitIndex = -1
	rf.lastApplied = -1

	rf.nextIndex = make([]int, len(rf.peers))
	rf.matchIndex = make([]int, len(rf.peers))

	// volatile non-RAFT variables
	rf.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	rf.serverState = Follower
	rf.electionTimer = time.NewTimer(MinElectionTimeout*2)
	rf.heartbeatTimer = time.NewTimer(HeartbeatTimeout)
	rf.ResetElectionTimer()
	rf.receivedVote = make([]bool, len(rf.peers))

	rf.requestVoteRequests = make(chan RequestVoteRequestData)
	rf.appendEntriesRequests = make(chan AppendEntriesRequestData)

	rf.requestVoteResponses = make(chan RequestVoteResponse)
	rf.appendEntriesResponses = make(chan AppendEntriesResponse)
	
	rf.startRequests = make(chan StartRequestData)
	rf.Verbosity = verbosity

	// initialize from state persisted before a crash
	rf.restoreState(rf.stateFilePath)

	// start the main loop
	go rf.Run()

	return rf
}

func (rf *Raft) ResetElectionTimer() {
	if !rf.electionTimer.Stop() && len(rf.electionTimer.C) != 0 {
		<-rf.electionTimer.C
	}
	time := MinElectionTimeout + time.Duration(rf.rng.Int63n(int64(MinElectionTimeout)))
	rf.electionTimer.Reset(time)
	rf.sendEvent(SetElectionTimeoutEvent{time})
}

func (rf *Raft) ResetHeartbeatTimer() {
	if !rf.heartbeatTimer.Stop() && len(rf.heartbeatTimer.C) != 0 {
		<-rf.heartbeatTimer.C
	}
	rf.heartbeatTimer.Reset(HeartbeatTimeout)
	rf.sendEvent(SetHeartbeatTimeoutEvent{HeartbeatTimeout})
}

func (rf *Raft) Run() {
	for true {
		select {
		case <-rf.electionTimer.C:
			if rf.serverState != Leader {
				rf.becomeCandidate()
			}
		case <-rf.heartbeatTimer.C:
			if rf.serverState == Leader {
				rf.sendAppendEntriesToAll()
			}
		case request := <-rf.requestVoteRequests:
			rf.handleRequestVote(request.args, request.reply)
			request.done <- true

		case request := <-rf.appendEntriesRequests:
			rf.handleAppendEntries(request.args, request.reply)
			request.done <- true
		case request := <-rf.startRequests:
			if rf.serverState != Leader {
				// first two return values should not be used
				request.response <- StartResponse{len(rf.log)-1, rf.currentTerm, false}
			} else {
				rf.log = append(rf.log, Log{rf.currentTerm, request.command})
				rf.matchIndex[rf.me] = rf.lastLogIndex()
				rf.persist()
				rf.onStateUpdated()

				rf.sendAppendEntriesToAll()
				//NOTE: internally we use 0-based-index, but the tests expect 1-based index
				request.response <- StartResponse{len(rf.log), rf.currentTerm, true}
			}
		case response := <-rf.requestVoteResponses:
			if rf.handleRequestVoteResponse(&response) && rf.serverState == Candidate {
				// got a vote
				rf.receivedVote[response.server] = true
				votes := 0
				for _, voted := range rf.receivedVote {
					if voted {
						votes++
					}
				}
				if votes > len(rf.peers)/2 {
					rf.becomeLeader()
				}
				rf.onStateUpdated()
			}

		case response := <-rf.appendEntriesResponses:
			rf.handleAppendEntriesResponse(&response)
		}
	}
}

// should be called when:
//   RequestVote has higher term
//   RequestVote Response has higher term
//   AppendEntries has higher term
func (rf *Raft) becomeFollower(term int, keepVotedFor bool) {
	if rf.Verbosity >= 2 && (rf.serverState != Follower || rf.currentTerm != term) {
		fmt.Printf("%v: Becoming Follower (keepVotedFor: %v)\n\tTerm: %v\n",
			rf.me, keepVotedFor, term)
	}

	if (rf.serverState != Follower || rf.currentTerm != term) {
		//prevents redundant events
		rf.onStateUpdated()
	}

	rf.serverState = Follower
	rf.currentTerm = term
	if !keepVotedFor {
		rf.votedFor = -1
	}
	rf.persist()
	rf.ResetElectionTimer()
}

func (rf *Raft) becomeLeader() {
	if rf.Verbosity >= 1 {
		fmt.Printf("%v: Becoming Leader\n\tTerm: %v\n", rf.me, rf.currentTerm)
	}

	rf.serverState = Leader

	// reinitialze leader volatile state
	for i, _ := range rf.nextIndex {
		rf.nextIndex[i] = rf.lastLogIndex() + 1
	}

	for i, _ := range rf.matchIndex {
		rf.matchIndex[i] = -1
	}

	// TODO: send no-op command with append entries to everyone (quick commit after leader election)
	rf.sendAppendEntriesToAll()
	rf.onStateUpdated()
}

func (rf *Raft) becomeCandidate() {
	if rf.Verbosity >= 2 {
		fmt.Printf("%v: Becoming Candidate\n\tNew term: %v\n", rf.me, rf.currentTerm + 1)
	}

	rf.serverState = Candidate
	// begin election
	rf.currentTerm++
	rf.ResetElectionTimer()

	// vote for self
	rf.votedFor = rf.me
	rf.persist()
	rf.receivedVote = make([]bool, len(rf.peers))
	rf.receivedVote[rf.me] = true

	// send requestVote to all peers
	requestVoteArgs := RequestVoteArgs{
		rf.currentTerm,
		rf.me,
		rf.lastLogIndex(),
		rf.lastLogTerm(),
	}

	for serverIndex := range rf.peers {
		if serverIndex != rf.me {
			go rf.sendRequestVote(serverIndex, requestVoteArgs)
		}
	}
	rf.onStateUpdated()
}

func (rf *Raft) sendAppendEntriesToAll() {
	if rf.Verbosity >= 3 {
		fmt.Printf("%v: Sending AppendEntries (lastLogIndex: %v)\n", rf.me, rf.lastLogIndex())
	}
	rf.ResetHeartbeatTimer()

	for serverIndex := range rf.peers {
		if serverIndex != rf.me {

			prevLogIndex := rf.nextIndex[serverIndex] - 1
			prevLogTerm := -1
			if prevLogIndex >= 0 {
				prevLogTerm = rf.log[prevLogIndex].Term
			}

			entries := rf.log[rf.nextIndex[serverIndex]:rf.lastLogIndex()+1]

			appendEntriesArgs := AppendEntriesArgs{
				rf.currentTerm,
				rf.me,
				prevLogIndex,
				prevLogTerm,
				entries,
				rf.commitIndex,
			}

			go rf.sendAppendEntries(serverIndex, appendEntriesArgs, rf.lastLogIndex())
		}
	}
}

// applies logs from lastApplied up to and including index
func (rf *Raft) applyLogs() {
	for rf.lastApplied < rf.commitIndex {
		if rf.Verbosity >= 1 {
			fmt.Printf("%v: Applying log index %v (term: %v, command %v)\n", rf.me, rf.lastApplied + 1, rf.log[rf.lastApplied + 1].Term, rf.log[rf.lastApplied + 1].Command)
		}

		rf.lastApplied++
		//NOTE: internally we use 0-based-index, but the tests expect 1-based index
		rf.applyCh <- ApplyMsg{rf.lastApplied + 1, rf.log[rf.lastApplied].Command, false, nil}
	}
}

func (rf *Raft) lastLogTerm() int {
	term := -1
	if len(rf.log) > 0 {
		term = rf.log[len(rf.log) - 1].Term
	}
	return term
}

func (rf *Raft) lastLogIndex() int {
	return len(rf.log) - 1
}

func (rf *Raft) sendEvent(event RaftEvent) {
	if rf.eventCh != nil {
		go func() {
			rf.eventCh<-event
		}()
	}
}
func (rf *Raft) onStateUpdated() {
	rf.sendEvent(StateUpdatedEvent{
		rf.serverState,
		rf.currentTerm,
		len(rf.log),
		rf.lastApplied,
		rf.log[maxInt(0,len(rf.log)-8):len(rf.log)],
		rf.votedFor,
		rf.receivedVote,
	})
}

