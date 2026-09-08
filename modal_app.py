"""
GOLD-MD bot ko Modal.com pe host karne ka deploy script.

Deploy: modal deploy modal_app.py   (repo root se)

FIXES (this deploy):
  1. .env / Redis creds / Storj creds ab main Go files me HARDCODED hain
     (storj.go, upstash.go, config.go) — .env skip hone pe bhi kaam karenge.
  2. Dockerfile ab .env ko runtime image me bhi copy karta hai (fallback).
  3. Persistent Volume /data — session DB ne redeploy ke baad bhi ZINDA rahegi
     (Redis quota khatam ho jaye tab bhi).
  4. Bot ka stdout + stderr ab Modal logs me stream hoga (subprocess inherit).

Cost: 0.5 core + 1GiB RAM 24/7 ≈ $22.7/month — $30 free credit me FIT.
"""

import modal

app = modal.App("gold-md-bot")

# Persistent volume — Redis fail ho tab bhi sessions / local DB safe.
vol = modal.Volume.from_name("goldmd-data", create_if_missing=True)

# Apna Dockerfile use karo (repo root me) — LibreOffice, ffmpeg,
# poppler sab khud install hote hain build k waqt.
# Bookworm me python3 + pip3 hote hain lekin `python`/`pip` alias nahi —
# Modal ka entrypoint unhe PATH pe chahta hai. Symlink add kar dete hain
# (bot ka apna python3 untouched rehta hai).
image = (
    modal.Image.from_dockerfile("Dockerfile")
    .apt_install("python3-pip")
    .run_commands(
        "ln -sf /usr/bin/python3 /usr/local/bin/python",
        "ln -sf /usr/bin/pip3 /usr/local/bin/pip",
    )
)

@app.function(
    image=image,
    secrets=[modal.Secret.from_name("goldmd-env")],
    # Persistent storage for sessions / local sqlite DB
    volumes={"/data": vol},
    # ~1 vCPU (0.5 physical core) + 1 GiB RAM guaranteed
    # RAM limit 1536 MB — antidelete media spikes safe
    cpu=0.5,
    memory=(1024, 1536),
    # Container HAMESHA alive — na sleep, na cold start
    min_containers=1,
    scaledown_window=3600,
)
@modal.web_server(2081, startup_timeout=180)
def run_bot():
    import subprocess, os
    # PORT env override karo taake panel web_server k port pe serve kare
    os.environ["PORT"] = "2081"
    # GOLDMD_DATA_DIR /data pe — persistent volume pe session DB.
    os.environ["GOLDMD_DATA_DIR"] = "/data/nexstore"
    os.environ["GOLDMD_SERVER_ID"] = "svr1"
    os.environ["GOLDMD_MAX_SESSIONS"] = "10"
    # .system truth values: billing limit 0.5 core + platform label
    os.environ["GOLDMD_CPU_LIMIT"] = "0.5 core (Modal billing limit)"
    os.environ["GOLDMD_PLATFORM"] = "Modal.com"
    # Bot ka binary (Dockerfile me /app/gold-md pe build hota hai).
    # CRITICAL: Popen (non-blocking) — subprocess.run pe web_server 300s me
    # timeout maar deta hai. Bot stdout+stderr inherit karta hai.
    subprocess.Popen(["./gold-md"])

    # Modal Volume writes function k return pe commit hote hain — web_server
    # kabhi return nahi karta, to container marne pe (redeploy/preempt/stop)
    # saara session data LOOT jata hai. Fix: har 60s volume commit karo.
    import threading, time
    def _commit_loop():
        while True:
            time.sleep(60)
            try:
                vol.commit()
            except Exception:
                pass
    threading.Thread(target=_commit_loop, daemon=True).start()
