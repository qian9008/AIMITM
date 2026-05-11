package main

import (
	"fmt"
	"sync"
	"time"
)

type Event struct {
	Msg string
}

var (
	clients    = make(map[chan Event]bool)
	clientsMu  sync.RWMutex
	eventChan  = make(chan Event, 10)
)

func broadcast() {
	for event := range eventChan {
		clientsMu.RLock()
		for ch := range clients {
			select {
			case ch <- event:
			default:
			}
		}
		clientsMu.RUnlock()
	}
}

func main() {
	go broadcast()

	// 模拟两个客户端连接
	c1 := make(chan Event, 1)
	c2 := make(chan Event, 1)
	
	clientsMu.Lock()
	clients[c1] = true
	clients[c2] = true
	clientsMu.Unlock()

	// 推送消息
	eventChan <- Event{Msg: "Optimize Success!"}
	
	// 验证两个客户端是否都收到了
	fmt.Printf("Client 1 received: %s\n", (<-c1).Msg)
	fmt.Printf("Client 2 received: %s\n", (<-c2).Msg)
	fmt.Println("Logic verification passed!")
}
