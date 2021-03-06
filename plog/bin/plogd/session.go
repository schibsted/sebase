// Copyright 2018 Schibsted

package main

import (
	"sync"
	"time"

	"github.com/schibsted/sebase/plog/internal/pkg/plogproto"
)

// SessionOutput is the connection between session and storage.
type SessionOutput interface {
	OpenDict(key string) (SessionOutput, error)
	OpenList(key string, dicts bool) (SessionOutput, error)
	Write(key string, value interface{}) error
	Close(proper, lastRef bool)
	ConfKey() string
}

// Session represents a session opened by a client.
type Session struct {
	store          *SessionStorage
	SessionType    plogproto.CtxType
	Writer         SessionOutput
	StartTimestamp time.Time
}

// Close removes the session. If proper is false a dignostic might be added to
// it first.
func (sess *Session) Close(proper bool) {
	sess.store.lock.Lock()
	lastRef := false
	if sess.SessionType == plogproto.CtxType_count {
		ck := sess.Writer.ConfKey()
		lastRef = sess.store.CountRefs[ck] == 1
		if lastRef {
			delete(sess.store.CountRefs, ck)
		} else {
			sess.store.CountRefs[ck]--
		}
	}
	sess.store.lock.Unlock()
	sess.Writer.Close(proper, lastRef)
}

// SessionStorage manages sessions.
type SessionStorage struct {
	lock      sync.Mutex
	CountRefs map[string]int
}

func (store *SessionStorage) newSession(output SessionOutput, stype plogproto.CtxType) (*Session, error) {
	store.lock.Lock()
	defer store.lock.Unlock()

	sess := &Session{store, stype, output, time.Now()}

	if stype == plogproto.CtxType_count {
		if store.CountRefs == nil {
			store.CountRefs = make(map[string]int)
		}
		store.CountRefs[output.ConfKey()]++
	}
	return sess, nil
}
