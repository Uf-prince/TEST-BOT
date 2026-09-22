// Local test server: serves public/index.html + /api/servers (same as Vercel).
const http = require('http');
const fs = require('fs');
const path = require('path');
const handler = require('../api/servers.js');

const root = path.resolve(__dirname, '..');
const port = process.env.PORT || 8080;

http.createServer((req, res) => {
  // Express-style helpers so the Vercel handler runs unchanged.
  res.status = (c) => { res.statusCode = c; return res; };
  res.json = (o) => {
    res.setHeader('Content-Type', 'application/json');
    res.end(JSON.stringify(o));
    return res;
  };

  if (req.url.startsWith('/api/servers')) {
    return handler(req, res);
  }
  const file = path.join(root, 'public', 'index.html');
  res.setHeader('Content-Type', 'text/html; charset=utf-8');
  res.end(fs.readFileSync(file));
}).listen(port, () => console.log('test server on http://localhost:' + port));
