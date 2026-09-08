package main

import (
	"sync"
	"time"
)

type AudioSession struct {
	Results []VideoResult
	Expiry  time.Time
}

var (
	audioSessions = make(map[string]*AudioSession)
	audioMu       sync.Mutex
)

func setAudioSession(jid string, results []VideoResult) {
	audioMu.Lock()
	defer audioMu.Unlock()
	audioSessions[jid] = &AudioSession{Results: results, Expiry: time.Now().Add(2 * time.Minute)} // 2-minute guaranteed validity
}

func getAudioSession(jid string) *AudioSession {
	audioMu.Lock()
	defer audioMu.Unlock()
	s, ok := audioSessions[jid]
	if !ok {
		return nil
	}
	if time.Now().After(s.Expiry) {
		delete(audioSessions, jid)
		return nil
	}
	return s
}

func clearAudioSession(jid string) {
	audioMu.Lock()
	defer audioMu.Unlock()
	delete(audioSessions, jid)
}

func clearMediaSessions(jid string) {
	clearVideoSession(jid)
	clearAudioSession(jid)
}
