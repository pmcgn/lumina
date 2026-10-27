package core

import "sync"

// Client is one connected browser. The transport (see package server) drains
// Out() and writes to the socket; the core only ever enqueues messages.
type Client struct {
	hub    *Hub
	out    chan []byte
	kick   func()
	mu     sync.Mutex
	subs   map[string]struct{}
	closed bool
}

// Out is the queue of encoded messages to write to the socket.
func (c *Client) Out() <-chan []byte { return c.out }

// Send enqueues a message. A client that cannot keep up is disconnected; it
// will reconnect and receive fresh snapshots.
func (c *Client) Send(msg []byte) {
	select {
	case c.out <- msg:
	default:
		c.hub.log.Warn("client too slow, disconnecting")
		c.kick()
	}
}

func (c *Client) subscribed(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.subs[id]
	return ok
}

func (c *Client) addSub(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	if _, ok := c.subs[id]; ok {
		return false
	}
	c.subs[id] = struct{}{}
	return true
}

func (c *Client) removeSub(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.subs[id]
	delete(c.subs, id)
	return ok
}

func (c *Client) subList() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.subs))
	for id := range c.subs {
		out = append(out, id)
	}
	return out
}
