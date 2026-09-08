package main

import (
	signalLogger "go.mau.fi/libsignal/logger"
)

// ============================================================================
// GOLD-MD — Silent Signal Library Logger (permanent)
// File: silent_signal_logger.go
// ============================================================================
// libsignal (whatsmeow ki crypto layer) ka apna built-in logger hai jo
// logger.Setup() na hone ki soorat me DEFAULT logger use karta hai — wo
// [ERROR]/[WARNING] lines (SessionCipher.go:319 "Unable to verify
// ciphertext mac" waghera) directly stdout pe fmt.Println kar deta hai,
// hamare silent logger system ko COMPLETELY bypass kar ke.
//
// Ye file ek no-op Loggable implementation install karti hai — ab libsignal
// ke saare internal logs (Debug/Info/Warning/Error) silently drop honge.
// Zero console output, zero file writes. Vendor code untouched.
//
// (Note: SessionCipher MAC mismatch errors normal hote hain — duplicate /
// out-of-order WhatsApp messages se aate hain, connection pe recover ho
// jate hain. Owner: zero logs policy.)
// ============================================================================

// silentSignalLogger libsignal ke logger.Logger interface (Loggable) ka
// no-op implementation hai — har level silently drop.
type silentSignalLogger struct{}

func (silentSignalLogger) Debug(caller, message string)   {}
func (silentSignalLogger) Info(caller, message string)    {}
func (silentSignalLogger) Warning(caller, message string) {}
func (silentSignalLogger) Warningf(caller string, format string, args ...any) {}
func (silentSignalLogger) Error(caller, message string)   {}
func (silentSignalLogger) Configure(settings string)      {}

// logger-package init: silent logger ko jaldi se install karo taake koi
// bhi libsignal call (boot me hi) na print ho sake.
func init() {
	var impl signalLogger.Loggable = silentSignalLogger{}
	signalLogger.Setup(&impl)
}
