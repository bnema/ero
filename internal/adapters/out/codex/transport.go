// Package codex provides pure Go types, helpers, and contracts for
// integrating with the Codex app-server JSON-RPC surface. It is the contract
// foundation for the bundled Codex review provider.
//
// In callback-only mode, the adapter connects directly to a running codex
// app-server via its unix control socket (WebSocket), performs the initialize
// handshake, and sends review messages via turn/start to a specific thread.
package codex
