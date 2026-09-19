#!/usr/bin/env node
// ============================================================================
//  GOLD-MD  ·  panel sync
// ============================================================================
//  src/panel.html is the SINGLE SOURCE OF TRUTH for the control-panel page.
//  It is:
//    1. embedded into the Go bot binary (src/panel.go → //go:embed panel.html)
//    2. copied to public/index.html for the Vercel static deploy
//
//  Run this after editing src/panel.html so both stay identical:
//      node scripts/sync-panel.js
// ============================================================================

const fs = require('fs');
const path = require('path');

const root = path.resolve(__dirname, '..');
const src = path.join(root, 'src', 'panel.html');
const dst = path.join(root, 'public', 'index.html');

if (!fs.existsSync(src)) {
  console.error('ERROR: missing ' + src);
  process.exit(1);
}

fs.mkdirSync(path.dirname(dst), { recursive: true });
fs.copyFileSync(src, dst);

const a = fs.readFileSync(src, 'utf8');
const b = fs.readFileSync(dst, 'utf8');
if (a !== b) {
  console.error('ERROR: copy mismatch');
  process.exit(1);
}

console.log('OK: src/panel.html → public/index.html (' + a.length + ' bytes)');
