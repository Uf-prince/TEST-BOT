package main

// ══════════════════════════════════════════════════════════════════════════
//   RAM PRELOAD GUARD  (owner order)
//
//   OWNER ORDER (Roman Urdu, verbatim intent):
//     "Bot restart ho crash ho band ho dubara on aye — ek guard bitha do jo
//      saara storage se data disk me le kar bhej de, phir sab disk se read ho,
//      phir wahi data RAM me rakhwao taake speed aur fast ho jaye. Jab RAM me
//      data na mile to disk check, wahan se load RAM. Disk khali ho to guard
//      inform karo — wo data load kar de ga disk me, phir disk se read, phir
//      RAM me. Antidelete aur antiedit jaise kaam kar rahe hain unhe waise hi
//      karne do. Aur agar user koi setting change/update/delete kare to foran
//      RAM update, disk bhi update, aur storage bhi update — taake RAM/disk se
//      data ure to storage me rahe. Guard is tarah lagana ke bot ki speed pe
//      0% asar dale, apna kaam bhi karta rahe."
//
//   ── 3-LAYER HIERARCHY (already wired, this file adds the RAM layer guard) ──
//
//     LAYER 1  RAM    (u.cache)        — 0ms, hot path (command dispatch,
//                                        guidance, mode/sudo/prefix checks)
//     LAYER 2  DISK   (nexstore/kvcache) — local files, ~0.1ms, survives
//                                        restart/crash (ephemeral-safe)
//     LAYER 3  STORJ  (S3)             — durable source of truth
//
//   READ  path: RAM → (miss) → DISK → (miss) → STORJ → populate DISK → RAM
//   WRITE path: RAM + DISK + STORJ  (all three, immediately — see storage.go
//               SetSetting/DelSetting/setAdd/setRem/SetPrefix)
//
//   This file adds:
//     1. dcPreloadRAM   — boot: disk ke saare hot-path keys RAM me preload.
//     2. dcStartRAMGuard — boot preload + continuous RAM guard (RAM khali ho
//                          to disk se foran reload — 0 network, sirf local
//                          files). 0% hot-path impact (background goroutine).
//
//   Antidelete/antiedit (msgs/ TTL data) is DELIBERATELY skipped from RAM —
//   it keeps working exactly as before (disk + Storj), owner order.
// ══════════════════════════════════════════════════════════════════════════

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// dcSkipRAMKey — bade / special keys jo RAM me NAHI rakhte (memory-safe).
//   - sessiondb  — MBs bade auth blobs (disk + Storj pe rehte hain).
//   - msgs:/msg: — antidelete/antiedit TTL data (owner: "waise hi karne do").
//   - goldmd:fleet: — fleet liveness keys (alag 60s refresher sambhalta hai).
func dcSkipRAMKey(key string) bool {
	if strings.Contains(key, "sessiondb") {
		return true
	}
	if strings.HasPrefix(key, "msgs:") || strings.HasPrefix(key, "msg:") {
		return true
	}
	if strings.HasPrefix(key, "goldmd:fleet:") {
		return true
	}
	return false
}

// dcPreloadRAM — disk-cache ke saare hot-path keys ko RAM me preload karo.
// Sirf LOCAL disk files padhta hai (0 network, 0 Storj bandwidth). Mapping:
//
//	hash  "settings:<jid>"  → "settings:<jid>:<field>" = value
//	set   "<key>"           → "setmembers:<key>"       = JSON array
//	string "<key>"          → "<key>"                  = value
//	missing "<key>"         → "<key>"                  = "\x00" sentinel
//
// Return: kitni RAM entries likhi gayin.
func dcPreloadRAM(u *Upstash) int {
	if !dcReady || u == nil {
		return 0
	}
	entries, err := os.ReadDir(dcDir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dcDir, e.Name()))
		if err != nil {
			continue
		}
		var st keyState
		if json.Unmarshal(data, &st) != nil {
			continue
		}
		// Purani file (key embedded nahi) — lazy read-through sambhalega.
		if st.Key == "" || dcSkipRAMKey(st.Key) {
			continue
		}
		switch st.Type {
		case "hash":
			for f, v := range st.Hash {
				u.cacheSet(st.Key+":"+f, v)
				n++
			}
		case "set":
			if len(st.Set) == 0 {
				u.cacheSet("setmembers:"+st.Key, "\x00")
			} else if b, err := json.Marshal(st.Set); err == nil {
				u.cacheSet("setmembers:"+st.Key, string(b))
			}
			n++
		case "string":
			u.cacheSet(st.Key, st.Str)
			n++
		case "missing":
			u.cacheSet(st.Key, "\x00")
			n++
		}
	}
	return n
}

// dcPopulateRAM — READ-THROUGH: jab RAM me data na mile aur disk (dcRead) se
// mil jaye, to usi value ko FORAN RAM me bhi likh do (owner order: "jab RAM me
// data na mile to disk check, wahan se load RAM"). Is se agli read 0ms ho jati
// hai. Sirf local RAM write — 0 network, 0 Storj bandwidth, hot path pe 0%
// asar (yeh sirf ek map write hai jo pehle se hi ho rahi read ke saath chalta
// hai). Antidelete/antiedit (msgs:/msg:) aur sessiondb deliberately skip.
func (u *Upstash) dcPopulateRAM(args []string, res json.RawMessage) {
	if u == nil || len(args) < 2 {
		return
	}
	op := strings.ToUpper(args[0])
	key := args[1]
	if dcSkipRAMKey(key) {
		return
	}
	switch op {
	case "GET":
		var s string
		if json.Unmarshal(res, &s) == nil {
			u.cacheSet(key, s)
		} else {
			u.cacheSet(key, "\x00")
		}
	case "SMEMBERS":
		var arr []string
		if json.Unmarshal(res, &arr) == nil {
			if len(arr) == 0 {
				u.cacheSet("setmembers:"+key, "\x00")
			} else if b, err := json.Marshal(arr); err == nil {
				u.cacheSet("setmembers:"+key, string(b))
			}
		}
	case "HGET":
		if len(args) >= 3 {
			var s string
			if json.Unmarshal(res, &s) == nil {
				u.cacheSet(key+":"+args[2], s)
			} else {
				u.cacheSet(key+":"+args[2], "\x00")
			}
		}
	case "HGETALL":
		var pairs []string
		if json.Unmarshal(res, &pairs) == nil {
			for i := 0; i+1 < len(pairs); i += 2 {
				u.cacheSet(key+":"+pairs[i], pairs[i+1])
			}
		}
	}
}

// dcStartRAMGuard — boot RAM preload + continuous RAM guard.
//
//  1. Boot settle (3s) — dcGuardLoad ko disk bharne do (agar disk khali thi).
//  2. dcPreloadRAM — disk → RAM (pehla command bhi 0ms).
//  3. Continuous (60s): agar RAM khali ho jaye (ClearCache / memory pressure /
//     kisi wajah se entries gayab) to foran DISK se reload — 0 network.
//
// Poora background goroutine — hot path pe 0% asar.
func dcStartRAMGuard(u *Upstash) {
	if !dcReady || u == nil {
		return
	}
	go func() {
		defer func() { _ = recover() }()
		// Boot settle: disk guard ko kaam karne do.
		time.Sleep(3 * time.Second)
		if n := dcPreloadRAM(u); n > 0 {
			InfoLog("RAM-GUARD: boot preload — %d keys RAM me (disk se, 0 network)", n)
		}
		t := time.NewTicker(60 * time.Second)
		defer t.Stop()
		for range t.C {
			if !dcReady {
				return
			}
			// RAM bhara hua hai → kuch na karo (0 kaam, 0 bandwidth).
			if u.CacheLen() > 0 {
				continue
			}
			if n := dcPreloadRAM(u); n > 0 {
				InfoLog("RAM-GUARD: RAM khali thi — disk se %d keys reload", n)
			}
		}
	}()
}
