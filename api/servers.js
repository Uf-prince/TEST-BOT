// ============================================================================
//  GOLD-MD  ·  Vercel serverless function  ·  /api/servers
// ============================================================================
//  This is the Vercel (Node.js) mirror of the Go panel's checkAllServers()
//  in src/panel.go.  It reads the SAME servers.json (single source of truth)
//  and live-checks every server's /health endpoint in parallel, returning the
//  exact same JSON shape the browser expects:
//
//      { "bot": "GOLD-MD", "max": 2,
//        "servers": [ { name, url, online, sessions, max, full, checkedAt } ] }
//
//  The Go bot keeps serving its own /api/servers from the embedded panel.
//  This function only exists so the panel page can also run as a static
//  deploy on Vercel (git-connected) with a working server dropdown.
// ============================================================================

const serversConfig = require('../servers.json');

const MAX_PER_SERVER =
  serversConfig && serversConfig.maxPerServer > 0
    ? serversConfig.maxPerServer
    : 2; // ⛔ HARDCODED 2 — DO NOT CHANGE (owner order)

const SERVERS =
  serversConfig && Array.isArray(serversConfig.servers)
    ? serversConfig.servers
    : [];

// checkOne hits <url>/health with a short timeout and returns a status object.
async function checkOne(entry) {
  const status = {
    name: entry.name,
    url: entry.url,
    online: false,
    sessions: 0,
    max: MAX_PER_SERVER,
    full: false,
    checkedAt: new Date().toISOString(),
  };

  const healthURL = String(entry.url || '').replace(/\/+$/, '') + '/health';
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 6000);

  try {
    const res = await fetch(healthURL, {
      signal: controller.signal,
      headers: { 'ngrok-skip-browser-warning': '1' },
    });
    if (res.ok) {
      const data = await res.json();
      const sessions = Number(data && data.sessions) || 0;
      status.online = true;
      status.sessions = sessions;
      status.full = sessions >= MAX_PER_SERVER;
    }
  } catch (_) {
    // offline / timeout / bad JSON → leave online=false
  } finally {
    clearTimeout(timer);
  }

  return status;
}

module.exports = async (req, res) => {
  // CORS — the panel may be opened from any origin.
  res.setHeader('Access-Control-Allow-Origin', '*');
  res.setHeader('Access-Control-Allow-Methods', 'GET, OPTIONS');
  res.setHeader('Access-Control-Allow-Headers', 'Content-Type');

  if (req.method === 'OPTIONS') {
    res.status(200).end();
    return;
  }

  try {
    const statuses = await Promise.all(SERVERS.map(checkOne));
    res.setHeader('Cache-Control', 'no-store');
    res.status(200).json({
      bot: 'GOLD-MD',
      max: MAX_PER_SERVER,
      servers: statuses,
    });
  } catch (err) {
    res.status(200).json({
      bot: 'GOLD-MD',
      max: MAX_PER_SERVER,
      servers: [],
      error: String((err && err.message) || err),
    });
  }
};
