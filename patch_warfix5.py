#!/usr/bin/env python3
# WAR-LOOP FIX part 5: reset circuit breaker when session is healthy.
import io, sys

SG = "src/session_guards.go"

def read(p):
    with io.open(p, "r", encoding="utf-8") as f:
        return f.read()

def write(p, s):
    with io.open(p, "w", encoding="utf-8") as f:
        f.write(s)

src = read(SG)

old = '''\t\t// Healthy (ya grace window): reset attempts so a future blip again gets fast retries.
\t\tm.resetAttempts(s.JID)
\t\treturn true'''
new = '''\t\t// Healthy (ya grace window): reset attempts so a future blip again gets fast retries.
\t\tm.resetAttempts(s.JID)
\t\t// WAR GUARD circuit breaker reset \u2014 session stable hai, agli war
\t\t// (agar aaye) fresh ginti se shuru ho.
\t\twarRetakeReset(s.JID)
\t\treturn true'''
if old not in src:
    print("ERR: reviveIfDead healthy block not found")
    sys.exit(1)
src = src.replace(old, new, 1)

write(SG, src)
print("OK: session_guards.go patched (circuit breaker reset)")
