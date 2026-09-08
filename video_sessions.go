package main

import (
	"sync"
	"time"
)

// VideoResult represents a single YouTube search result (internal type).
type VideoResult struct {
	Title     string
	URL       string
	Thumbnail string
	Duration  string
}

// VideoSession stores search results for a user pending number selection.
type VideoSession struct {
	Results []VideoResult
	Expiry  time.Time
	Video2  bool // true when the list came from .video2 (turbo engine)
	HD      bool // true when the user asked for HD quality
}

var (
	videoSessions = make(map[string]*VideoSession)
	sessionsMu    sync.Mutex
)

func setVideoSession(jid string, results []VideoResult) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	videoSessions[jid] = &VideoSession{
		Results: results,
		Expiry:  time.Now().Add(2 * time.Minute), // 2-minute guaranteed validity
	}
}

// setVideoSession2 stores a .video2 (turbo) session so number picks route
// back through the video2 engine, with HD mode when requested.
func setVideoSession2(jid string, results []VideoResult, hd bool) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	videoSessions[jid] = &VideoSession{
		Results: results,
		Expiry:  time.Now().Add(2 * time.Minute),
		Video2:  true,
		HD:      hd,
	}
}

func getVideoSession(jid string) *VideoSession {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	sess, ok := videoSessions[jid]
	if !ok {
		return nil
	}
	if time.Now().After(sess.Expiry) {
		delete(videoSessions, jid)
		return nil
	}
	return sess
}

func clearVideoSession(jid string) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	delete(videoSessions, jid)
}
