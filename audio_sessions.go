package main

import (
	"sync"
	"time"
)

type AudioSession struct {
	Results []VideoResult
	Expiry  time.Time
	Play2   bool // true when the list came from .play2 (turbo audio engine)
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

func setAudioSession2(jid string, results []VideoResult, play2 bool) {
	audioMu.Lock()
	defer audioMu.Unlock()
	audioSessions[jid] = &AudioSession{Results: results, Expiry: time.Now().Add(2 * time.Minute), Play2: play2}
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
