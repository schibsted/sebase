// Copyright 2018 Schibsted

package plogproto

import (
	"bufio"
	"encoding/binary"
	fmt "fmt"
	"io"
	"net"
	"net/url"
	"sync"

	proto "github.com/golang/protobuf/proto"
)

//go:generate protoc --go_out=. plog.proto

const (
	// DefaultSock is the default path to listen on / connect to.
	DefaultSock = "/run/plog/plog.sock"
)

// Listener is a convenience listener type. Set seqpacket to true in case
// that's the socket type used.
type Listener struct {
	net.Listener
	Seqpacket bool
}

// Accept on the contained listener and wrap the returned connection.
func (l Listener) Accept() (*Reader, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return NewReader(c, l.Seqpacket), nil
}

// NewClientConn parses sock either as a URL or a path and opens a connection
// there.  If sock is empty string the DefaultSock value is used.
// The underlying WriteCloser can be cast to net.Conn if necessary.
func NewClientConn(sock string) (*Writer, error) {
	surl, err := url.Parse(sock)
	if err != nil {
		return nil, err
	}
	if surl.Scheme == "" {
		if surl.Host != "" {
			surl.Scheme = "tcp"
		} else {
			surl.Scheme = "unix"
		}
	}
	if surl.Path == "" {
		surl.Path = DefaultSock
	}
	var conn net.Conn
	switch surl.Scheme {
	case "tcp", "tcp4", "tcp6":
		conn, err = net.Dial(surl.Scheme, surl.Host)
	case "unix", "unixpacket":
		conn, err = net.Dial(surl.Scheme, surl.Path)
	default:
		return nil, fmt.Errorf("Bad scheme in PLOG_SOCKET url")
	}
	return NewWriter(conn, surl.Scheme == "unixpacket"), err
}

// Reader for receiving plog messages. Has Closer for easier use.
type Reader struct {
	br *bufio.Reader
	io.Closer
	Seqpacket bool
}

// NewReader creates a new reader for plog messages.
func NewReader(rc io.ReadCloser, seqpacket bool) *Reader {
	return &Reader{bufio.NewReader(rc), rc, seqpacket}
}

// Receive overwrites *msg with a message received from the connection.
func (r *Reader) Receive(msg *Plog) error {
	var l uint32
	if err := binary.Read(r.br, binary.BigEndian, &l); err != nil {
		return err
	}
	data := make([]byte, l)
	_, err := io.ReadFull(r.br, data)
	if err != nil {
		return err
	}
	return proto.Unmarshal(data, msg)
}

// Writer for sending plog messages.
// Has closer for easier use.
type Writer struct {
	bw *bufio.Writer
	io.Closer
	Seqpacket bool
	sync.Mutex
}

// NewWriter creates a new writer for plog messages.
func NewWriter(wc io.WriteCloser, seqpacket bool) *Writer {
	return &Writer{bw: bufio.NewWriter(wc), Closer: wc, Seqpacket: seqpacket}
}

// Send *msg on the connection.
func (w *Writer) Send(msg *Plog) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}

	// Use a simple len prefix.
	w.Lock()
	defer w.Unlock()
	l := uint32(len(data))
	if err := binary.Write(w.bw, binary.BigEndian, l); err != nil {
		return err
	}
	// bufio.Writer always write all or return an error.
	_, err = w.bw.Write(data)
	if err != nil {
		return err
	}
	// Always flush, we're not allowed to store things on this side.
	return w.bw.Flush()
}

// SendOpen is a convenience wrapper for sending only the open message.
func (w *Writer) SendOpen(ctxID uint64, msg *OpenContext) error {
	return w.Send(&Plog{CtxId: &ctxID, Open: msg})
}

// SendClose is a convenience wrapper for sending only the close message.
func (w *Writer) SendClose(ctxID uint64) error {
	cl := true
	return w.Send(&Plog{CtxId: &ctxID, Close: &cl})
}

// SendMessage is a convenience wrapper for sending only message messages.
func (w *Writer) SendMessage(ctxID uint64, msg ...*PlogMessage) error {
	return w.Send(&Plog{CtxId: &ctxID, Msg: msg})
}

// SendKeyValue is a convenience wrapper for sending a single message.
func (w *Writer) SendKeyValue(ctxID uint64, key string, value []byte) error {
	msg := PlogMessage{Key: &key, Value: value}
	return w.SendMessage(ctxID, &msg)
}
