// Broadcast over channels.
package bcast

/*
   bcast package for Go. Broadcasting on a set of channels.

   Copyright © 2013 Alexander I.Grafov <grafov@gmail.com>.
   All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.
*/

import (
	"sync"
	"time"
)

// Internal structure to pack messages together with info about sender.
type Message struct {
	sender  chan interface{}
	payload interface{}
}

// Represents member of broadcast group.
type Member struct {
	group *Group           // send messages to others directly to group.In
	In    chan interface{} // (public) get messages from others to own channel
}

// Represents broadcast group.
type Group struct {
	l     sync.Mutex
	in    chan Message       // receive broadcasts from members
	out   []chan interface{} // broadcast messages to members
	count int
	close chan bool
}

// Create new broadcast group.
func NewGroup() *Group {
	in := make(chan Message)
	close := make(chan bool)
	return &Group{in: in, close: close}
}

func (r *Group) MemberCount() int {
	return r.count
}

func (r *Group) Members() []chan interface{} {
	r.l.Lock()
	res := r.out[:]
	r.count = len(r.out)
	r.l.Unlock()
	return res
}

func (r *Group) Add(out chan interface{}) {
	r.l.Lock()
	r.out = append(r.out, out)
	r.count = len(r.out)
	r.l.Unlock()
	return
}

func (r *Group) Remove(received Message) {
	r.l.Lock()
	for i, addr := range r.out {
		if addr == received.payload.(Member).In && received.sender == received.payload.(Member).In {
			r.out = append(r.out[:i], r.out[i+1:]...)
			r.count = len(r.out)
			break
		}
	}
	r.l.Unlock()
	return
}

// Close the group immediately
func (r *Group) Close() {
	r.close <- true
}

// Broadcast messages received from one group member to others.
// If incoming messages not arrived during `totalTimeout` then function returns.
// Will wait `messageTimeout` for message to be read by a group member
func (r *Group) BroadcastFor(totalTimeout time.Duration, messageTimeout time.Duration) {
	if totalTimeout <= 0 {
		r.Broadcast()
		return
	}
	// set default timeout of 1 hour
	if messageTimeout <= 0 {
		messageTimeout = time.Duration(time.Hour)
	}
	for {
		select {
		case received := <-r.in:
			switch received.payload.(type) {
			default: // receive a payload and broadcast it
				for _, member := range r.Members() {
					if received.sender != member { // not return broadcast to sender
						go func(out chan interface{}, received *Message) { // non blocking
							select {
							case out <- received.payload:
								return
							case <-time.After(messageTimeout): // avoid ghost routines
								return
							}
						}(member, &received)
					}
				}
			}
		case <-time.After(totalTimeout):
			return
		case <-r.close:
			return
		}
	}
}

// Broadcast messages received from one group member to others.
// See https://github.com/grafov/bcast/issues/4 for rationale.
func (r *Group) Broadcast() {
	messageTimeout := time.Duration(time.Hour)
	for {
		select {
		case <-r.close:
			return
		case received := <-r.in:
			switch received.payload.(type) {
			default: // receive a payload and broadcast it
				for _, member := range r.Members() {
					if received.sender != member { // not return broadcast to sender
						go func(out chan interface{}, received *Message) { // non blocking
							select {
							case out <- received.payload:
								return
							case <-time.After(messageTimeout): // avoid ghost routines
								return
							}
						}(member, &received)
					}
				}
			}
		}
	}
}

// Broadcast message to all group members.
func (r *Group) Send(val interface{}) {
	r.in <- Message{sender: nil, payload: val}
}

// Join new member to broadcast.
func (r *Group) Join() *Member {
	out := make(chan interface{})
	r.Add(out)
	//r.out = append(r.out, out)
	return &Member{group: r, In: out}
}

// Unjoin member from broadcast group.
func (r *Member) Close() {
	r.group.Remove(Message{sender: r.In, payload: *r})
	//r.group.in <- Message{sender: r.In, payload: *r} // broadcasting of self means member closing
}

// Broadcast message from one member to others except sender.
func (r *Member) Send(val interface{}) {
	r.group.in <- Message{sender: r.In, payload: val}
}

// Get broadcast message.
// As alternative you may get it from `In` channel.
func (r *Member) Recv() interface{} {
	return <-r.In
}
