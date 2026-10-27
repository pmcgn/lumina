package core

// Message type names of the WebSocket protocol (Envelope.Type). Not generated
// to TypeScript: the frontend uses these names as plain strings.

// Client -> server message types.
const (
	MsgHello       = "hello"
	MsgSubscribe   = "subscribe"
	MsgUnsubscribe = "unsubscribe"
	MsgAction      = "action"
	MsgPing        = "ping"
)

// Server -> client message types.
const (
	MsgWelcome      = "welcome"
	MsgConfig       = "config"
	MsgSnapshot     = "snapshot"
	MsgUpdate       = "update"
	MsgActionResult = "action_result"
	MsgError        = "error"
	MsgPong         = "pong"
)
