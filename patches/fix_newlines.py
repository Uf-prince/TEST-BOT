#!/usr/bin/env python3
# Fix: literal newlines inside Go string -> \n escapes
path = "fleet.go"
src = open(path, encoding="utf-8").read()

start = src.find('\ttext := "*')
end = src.find('"\\n\\n" +', start)
# locate the end of the text assignment: line '✅ Apne pas rakh liya...koi farak nahi."'
anchor = '"\\u2705 Apne pas rakh liya'
i = src.find("Apne pas rakh liya", start)
assert i > 0, "anchor not found"
# find the closing quote + newline after that phrase
j = src.find('nahi."', i)
assert j > 0, "close not found"
j += len('nahi."')

old_block = src[start:j]
# verify it contains real newlines (the bug)
assert "\n\n" in old_block or "\n" in old_block.strip('"'), "no literal newline found?"

new_block = (
    '\ttext := "*── SESSION FAILOVER ──*\\n\\n" +\n'
    '\t\t"\\u26a1 Purana server band ho gaya tha (" + dead + ").\\n" +\n'
    '\t\t"\\u2705 Bot ab dusre server pe *online* hai — session *reconnect ho gaya*\\n" +\n'
    '\t\t"\\u2139\\ufe0f New Server: " + srv + "\\n\\n" +\n'
    '\t\t"\\u2705 Apne pas rakh liya — ab jaise pehle hi use karo, koi farak nahi."'
)
src = src[:start] + new_block + src[j:]
open(path, "w", encoding="utf-8").write(src)
print("newline escapes fixed OK")
