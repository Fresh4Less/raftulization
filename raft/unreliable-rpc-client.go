package raft

import (
	"fmt"
	"time"
	"net/rpc"
	"sync"
)


type UnreliableRpcClient struct {
	Address string
	RetryCount int
	RetryTimeout time.Duration
	Client *rpc.Client
	lock *sync.Mutex
}

func NewUnreliableRpcClient(address string, retryCount int, retryTimeout time.Duration) *UnreliableRpcClient {
	urc := UnreliableRpcClient{address, retryCount, retryTimeout, nil, &sync.Mutex{}}
	urc.getClient() //try to estabilish a connection immediately
	return &urc
}

func (urc *UnreliableRpcClient) Call(serviceMethod string, args interface{}, reply interface{}) bool {

	for i := urc.RetryCount; i > 0; i-- {
		success := func() bool {
			client := urc.getClient()
			if client == nil {
				return false
			}

			err := client.Call(serviceMethod, args, reply)
			if err != nil {
				fmt.Printf("Failed to call RPC %v: %v\n", serviceMethod, err)
				// assume the connection was closed and reset Client
				urc.Client = nil
				return false
			}
			return true
		}()

		if success {
			return true
		} else {
			time.Sleep(urc.RetryTimeout)
		}
	}
	return false
	
}

func (urc *UnreliableRpcClient) getClient() *rpc.Client {
	urc.lock.Lock()
	defer urc.lock.Unlock()

	if urc.Client == nil {
		client, err := rpc.DialHTTP("tcp", urc.Address)
		if err != nil {
			fmt.Printf("Failed to dial %v: %v\n", urc.Address, err)
		} else {
			fmt.Printf("Connected to %v\n", urc.Address)
			urc.Client = client
		}
	}
	return urc.Client
}
