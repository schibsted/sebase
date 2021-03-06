// Copyright 2018 Schibsted

package main

import (
	"bytes"
	"context"
	"log"
	"net"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/schibsted/sebase/plog/pkg/plogd"
)

type netmsg int

const (
	data       netmsg = iota
	disconnect        = iota
	shutdown          = iota
)

// NetWriter is used to send logs over the network.
type NetWriter struct {
	network, address string
	Conn             net.Conn
	LocalIP          string
	bytes.Buffer
	signal chan netmsg
	data   chan []byte
	sync.Mutex
}

func (wr *NetWriter) eofReader() {
	for {
		_, err := wr.Conn.Read([]byte{0})
		if err != nil {
			break
		}
	}
	wr.signal <- disconnect
}

// Connect tries to connect and starts the thread writing the buffer if
// successful.  It will return nil on connection refused even though no
// connection is established.
func (wr *NetWriter) Connect() error {
	var err error
	wr.Conn, err = net.Dial(wr.network, wr.address)
	if err != nil {
		if operr, ok := err.(*net.OpError); ok {
			err = operr.Err
		}
		if scerr, ok := err.(*os.SyscallError); ok {
			err = scerr.Err
		}
		if err == syscall.ECONNREFUSED {
			time.Sleep(500 * time.Millisecond)
			err = nil
		}
	}
	if err != nil {
		log.Printf("net_writer: connect(%s, %s): %v", wr.network, wr.address, err)
		return err
	}
	if wr.Conn != nil {
		wr.LocalIP = wr.Conn.LocalAddr().(*net.TCPAddr).IP.String()
		go wr.eofReader()
	}
	return nil
}

func netWriterLoop(wr *NetWriter) {
	running := true
	for running || wr.Buffer.Len() > 0 {
		wait := running
		wr.Mutex.Lock()
		for more := true; more; {
			select {
			case d := <-wr.data:
				wr.Buffer.Write(d)
			default:
				more = false
			}
		}
		if wr.Buffer.Len() > 0 {
			wait = false
			if wr.Conn == nil {
				// Unlock while connecting
				wr.Mutex.Unlock()
				wr.Connect()
				wr.Mutex.Lock()
			} else {
				_, err := wr.Buffer.WriteTo(wr.Conn)
				if err != nil {
					// Trigger eofReader
					wr.Conn.Close()
					wait = true
				}
			}
		}
		wr.Mutex.Unlock()
		if wait {
			select {
			case disc, ok := <-wr.signal:
				if !ok || disc == shutdown {
					running = false
				} else if disc == disconnect {
					wr.Conn.Close()
					wr.Conn = nil
				}
			case d := <-wr.data:
				wr.Mutex.Lock()
				wr.Buffer.Write(d)
				wr.Mutex.Unlock()
			}
		}
	}
	if wr.Conn != nil {
		wr.Conn.Close()
	}
}

// NewNetWriter creates a writer for the given address.
// It will try to connect but connection refused is ignored, while other errors
// are reported.
func NewNetWriter(network, address string) (*NetWriter, error) {
	wr := &NetWriter{network: network, address: address, signal: make(chan netmsg, 1), data: make(chan []byte, 1024)}
	// Do the first connect here to detect more serious problems
	if err := wr.Connect(); err != nil {
		close(wr.signal)
		return nil, err
	}
	go netWriterLoop(wr)
	return wr, nil
}

// Close signals the writer to close. It's asynchronous.
func (wr *NetWriter) Close() error {
	wr.signal <- shutdown
	return nil
}

// WriteMessage implements the storage output interface. It queues the message
// in the output buffer.
func (wr *NetWriter) WriteMessage(ctx context.Context, msg plogd.LogMessage) {
	// msg.Host will be empty string here, add it.
	if msg.Host == "" {
		msg.Host = wr.LocalIP
	}
	body, err := msg.MarshalJSON()
	if err != nil {
		panic(err)
	}
	body = append(body, '\n')
	wr.data <- body
}

// ResetBuffer replaces the output buffer, returning the previous contents.
func (wr *NetWriter) ResetBuffer(newbuf []byte) []byte {
	wr.Mutex.Lock()
	ret := wr.Buffer.Bytes()
	wr.Buffer.Reset()
	if newbuf != nil {
		wr.Buffer.Write(newbuf)
	}
	wr.Mutex.Unlock()
	return ret
}
