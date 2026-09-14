package main

// ============================================================================
// GOLD-MD — logo1..logo1000 hidden command registrations (main package)
// File: logo1000_main.go
// ============================================================================
// OWNER ORDER: logo1..logo1000 .menu aur .fullmenu DONO me kabhi nahi
// dikhne chahiye. Isliye ye Commands map me direct register hote hain
// (gold-cmds registry me nahi) + hiddenCommands set me hain:
//   .menu    → hiddenCommands filter (manager.go:1794) unhe skip karta hai
//   .fullmenu → sirf gold-cmds registry + fmCoreCommands scan karta hai —
//               main-package Commands map scan me coreCommandCategory/
//               coreCommandDesc use hota hai, jo in naam ko pehchan kar
//               AI & MEDIA category me daal deta hai (warna OTHER me).
//               OWNER ORDER: in dono ko pehchan kar SKIP karna hai.
// Handler: goldcmds.LogoRunN(n) — 15-key Agnes pool + 1000 designs engine.
// ============================================================================

import (
	"fmt"

	"go.mau.fi/whatsmeow/types"

	goldcmds "gold-md/gold-cmds"
)

// registerLogo1000Commands registers logo1..logo1000 in the main-package
// Commands map (hidden from both menus per owner order).
func registerLogo1000Commands() {
	for n := 1; n <= goldcmds.LogoCount; n++ {
		n := n
		name := fmt.Sprintf("logo%d", n)
		RegisterCommand(name, func(s *Session, info types.MessageInfo, args []string, prefix string) {
			beginCmdBusy()
			func() {
				defer endCmdBusy()
				goldcmds.LogoRunN(&bridge{s: s}, info, args, prefix, n)
			}()
		})
		hiddenCommands[name] = true
	}
}

func init() {
	registerLogo1000Commands()
}
