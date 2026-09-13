#!/usr/bin/env python3
# FIRST-13 SERVER LINKS FIX (owner ki updated list):
#   SERVER 5: omil -> botxd2
#   SERVER 6-8: svrr7/8/9 -> svrr10/11/12
#   SERVER 9-11: svrr10/11/12 -> svrr7/8/9
#   (SERVER 1-4, 12, 13 aur SERVER 14-200 xsvr5..xsvr191 UNCHANGED)
# Files: servers.json, panel.go (defaultServers fallback), fleet_failover_test.go (omil test case)
import json, sys

NEW5 = "https://gold-md-botxd2.onrender.com"   # SERVER 5
NEW6 = "https://gold-md-svrr10.onrender.com"   # SERVER 6
NEW7 = "https://gold-md-svrr11.onrender.com"   # SERVER 7
NEW8 = "https://gold-md-svrr12.onrender.com"   # SERVER 8
NEW9 = "https://gold-md-svrr7.onrender.com"    # SERVER 9
NEW10 = "https://gold-md-svrr8.onrender.com"   # SERVER 10
NEW11 = "https://gold-md-svrr9.onrender.com"   # SERVER 11

def die(msg):
    print("PATCH-FAIL:", msg); sys.exit(1)

# ---- 1) servers.json ----
raw = open("servers.json").read()
data = json.loads(raw)
srvs = data["servers"]
assert len(srvs) == 200, f"servers count {len(srvs)} != 200"
def want(i, name, url):
    assert srvs[i]["name"] == name, f"idx{i} name {srvs[i]['name']} != {name}"
    assert srvs[i]["url"] == url, f"idx{i} url {srvs[i]['url']} != {url}"
# pre-state assert (rotation-safe: old URLs exactly yahi hone chahiye)
want(4,  "SERVER 5",  "https://gold-md-omil.onrender.com")
want(5,  "SERVER 6",  "https://gold-md-svrr7.onrender.com")
want(6,  "SERVER 7",  "https://gold-md-svrr8.onrender.com")
want(7,  "SERVER 8",  "https://gold-md-svrr9.onrender.com")
want(8,  "SERVER 9",  "https://gold-md-svrr10.onrender.com")
want(9,  "SERVER 10", "https://gold-md-svrr11.onrender.com")
want(10, "SERVER 11", "https://gold-md-svrr12.onrender.com")
want(11, "SERVER 12", "https://gold-md-svrr13.onrender.com")
want(12, "SERVER 13", "https://gold-md-svrr13.onrender.com")
# apply
srvs[4]["url"], srvs[5]["url"], srvs[6]["url"] = NEW5, NEW6, NEW7
srvs[7]["url"], srvs[8]["url"], srvs[9]["url"] = NEW8, NEW9, NEW10
srvs[10]["url"] = NEW11
# untouched anchors
want(0, "SERVER 1", "https://gold-md-xsvr1.onrender.com")
want(13, "SERVER 14", "https://gold-md-xsvr5.onrender.com")
want(199, "SERVER 200", "https://gold-md-xsvr191.onrender.com")
# comment update
data["_comment"] = ("GOLD-MD multi-server panel + hidden .server menu config. 200 Render free deployments "
    "(512MB RAM / 5GB bandwidth each, max 2 pairings per server). SERVER 1-13 owner ki updated list se "
    "(xsvr1-4, botxd2, svrr10/11/12, svrr7/8/9, svrr13, svrr13). SERVER 14-200 = xsvr5-xsvr191 (auto-generated). "
    "Owner baad me on kar dega \u2014 off servers .server report me STOPPED dikhenge.")
out = json.dumps(data, indent=2) + "\n"
open("servers.json", "w").write(out)
print("servers.json OK: 200 servers, SERVER 5-11 updated, 14-200 untouched")

# ---- 2) panel.go defaultServers fallback ----
src = open("panel.go").read()
pairs = [
    ('{Name: "SERVER 5", URL: "https://gold-md-omil.onrender.com"},',
     '{Name: "SERVER 5", URL: "https://gold-md-botxd2.onrender.com"},'),
    ('{Name: "SERVER 6", URL: "https://gold-md-svrr7.onrender.com"},',
     '{Name: "SERVER 6", URL: "https://gold-md-svrr10.onrender.com"},'),
    ('{Name: "SERVER 7", URL: "https://gold-md-svrr8.onrender.com"},',
     '{Name: "SERVER 7", URL: "https://gold-md-svrr11.onrender.com"},'),
    ('{Name: "SERVER 8", URL: "https://gold-md-svrr9.onrender.com"},',
     '{Name: "SERVER 8", URL: "https://gold-md-svrr12.onrender.com"},'),
    ('{Name: "SERVER 9", URL: "https://gold-md-svrr10.onrender.com"},',
     '{Name: "SERVER 9", URL: "https://gold-md-svrr7.onrender.com"},'),
    ('{Name: "SERVER 10", URL: "https://gold-md-svrr11.onrender.com"},',
     '{Name: "SERVER 10", URL: "https://gold-md-svrr8.onrender.com"},'),
    ('{Name: "SERVER 11", URL: "https://gold-md-svrr12.onrender.com"},',
     '{Name: "SERVER 11", URL: "https://gold-md-svrr9.onrender.com"},'),
]
# idempotency: sab new already present? skip
if all(new in src for _, new in pairs):
    print("panel.go: already patched, SKIP")
else:
    # rotation problem: purane svrr7/8/9 wale SERVER 9/10/11 lines ko pehle temp-token se replace karo
    # (varna 'SERVER 6' ka naya URL 'SERVER 9' ke purane se match ho sakta hai order me)
    # trick: UNIQUE anchors = Name+URL dono, isliye seedha pairs apply karna safe hai —
    # lekin order ulta karte hain (9,10,11 pehle) taake collision na ho
    for old, new in reversed(pairs):
        if src.count(old) != 1:
            die(f"panel.go anchor count {src.count(old)} != 1: {old[:60]}")
        src = src.replace(old, new)
    open("panel.go", "w").write(src)
    print("panel.go OK: defaultServers SERVER 5-11 updated")

# ---- 3) fleet_failover_test.go: omil test case -> botxd2 ----
tst = open("fleet_failover_test.go").read()
old_tc = '{"https://gold-md-omil.onrender.com/", "5"},'
new_tc = '{"https://gold-md-botxd2.onrender.com/", "5"},'
if new_tc in tst:
    print("fleet_failover_test.go: already patched, SKIP")
elif tst.count(old_tc) == 1:
    open("fleet_failover_test.go", "w").write(tst.replace(old_tc, new_tc))
    print("fleet_failover_test.go OK: omil -> botxd2 (SERVER 5)")
else:
    die(f"test anchor count {tst.count(old_tc)} != 1")

print("ALL PATCHES OK")
