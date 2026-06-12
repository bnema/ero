package codex

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ---------------------------------------------------------------------------
// JSON-RPC message types
//
// Codex app-server uses JSON-RPC 2.0 semantics with the "jsonrpc":"2.0"
// header omitted on the wire. All communication is newline-delimited JSON
// (--stdio mode) or single JSON objects per WebSocket frame (proxy/unix).
// ---------------------------------------------------------------------------

// Message is a raw JSON-RPC message. Use DecodeMessage to parse an incoming
// byte sequence into either a Request, Response, or Notification.
type Message struct {
	// ID present  → Request (object or number) or Response (same id echoed)
	// ID absent   → Notification
	ID json.RawMessage `json:"id,omitempty"`
	// Method is present for Requests and Notifications, absent for Responses.
	Method string `json:"method,omitempty"`
	// Params carries request/notification parameters.
	Params json.RawMessage `json:"params,omitempty"`
	// Result carries a successful response payload.
	Result json.RawMessage `json:"result,omitempty"`
	// Error carries a JSON-RPC error object.
	Error *RPCError `json:"error,omitempty"`
}

// RPCError is a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error implements the error interface.
func (e *RPCError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message == "" {
		return fmt.Sprintf("JSON-RPC error %d", e.Code)
	}
	return fmt.Sprintf("JSON-RPC error %d: %s", e.Code, e.Message)
}

// Well-known JSON-RPC error codes used by codex app-server.
const (
	// ErrCodeParseError is the standard JSON-RPC -32700.
	ErrCodeParseError = -32700
	// ErrCodeInvalidRequest is the standard JSON-RPC -32600.
	ErrCodeInvalidRequest = -32600
	// ErrCodeMethodNotFound is the standard JSON-RPC -32601.
	ErrCodeMethodNotFound = -32601
	// ErrCodeInvalidParams is the standard JSON-RPC -32602.
	ErrCodeInvalidParams = -32602
	// ErrCodeInternalError is the standard JSON-RPC -32603.
	ErrCodeInternalError = -32603

	// ErrCodeOverloaded is the app-server custom error sent when request
	// ingress is saturated. The server message is "Server overloaded; retry
	// later." Clients should treat this as retryable with exponential backoff
	// and jitter.
	ErrCodeOverloaded = -32001

	// ErrCodeNotInitialized is sent when a request arrives on a connection
	// before the initialize/initialized handshake has completed.
	ErrCodeNotInitialized = -32002

	// ErrCodeAlreadyInitialized is sent when a second initialize request is
	// received on the same connection.
	ErrCodeAlreadyInitialized = -32003
)

// Standard JSON-RPC error messages returned by the server.
const (
	ErrMsgOverloaded         = "Server overloaded; retry later."
	ErrMsgNotInitialized     = "Not initialized"
	ErrMsgAlreadyInitialized = "Already initialized"
)

// MessageKind classifies a raw JSON-RPC message.
type MessageKind int

const (
	// MsgRequest is a JSON-RPC request with an id and a method.
	MsgRequest MessageKind = iota
	// MsgNotification is a JSON-RPC notification (method present, no id).
	MsgNotification
	// MsgResponse is a JSON-RPC response (id present, no method).
	MsgResponse
	// MsgInvalid is a message that does not conform to JSON-RPC 2.0
	// structural rules (e.g. id:null with a method present).
	MsgInvalid
)

// String returns a human-readable label for the message kind.
func (k MessageKind) String() string {
	switch k {
	case MsgRequest:
		return "request"
	case MsgNotification:
		return "notification"
	case MsgResponse:
		return "response"
	case MsgInvalid:
		return "invalid"
	default:
		return "unknown"
	}
}

// ClassifyMessage determines the kind of a raw JSON-RPC message based on
// the presence of id and method fields.
func ClassifyMessage(msg Message) MessageKind {
	hasID := len(msg.ID) > 0 && string(msg.ID) != "null"
	hasMethod := msg.Method != ""
	hasNullID := len(msg.ID) > 0 && string(msg.ID) == "null"
	switch {
	case hasNullID && hasMethod:
		return MsgInvalid
	case hasMethod && !hasID:
		return MsgNotification
	case hasMethod && hasID:
		return MsgRequest
	case !hasMethod:
		return MsgResponse
	default:
		return MsgResponse
	}
}

// DecodeMessage parses a raw JSON byte slice into a Message struct. It does
// not validate the message beyond JSON syntax — use ClassifyMessage to
// determine the kind.
func DecodeMessage(data []byte) (Message, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return Message{}, fmt.Errorf("codex json-rpc: decode message: %w", err)
	}
	return msg, nil
}

// EncodeRequest serializes a JSON-RPC request with the given id, method, and
// params. Params may be nil for methods that take no parameters.
func EncodeRequest(id json.RawMessage, method string, params any) ([]byte, error) {
	rawParams, err := encodeParams(params)
	if err != nil {
		return nil, err
	}
	msg := Message{
		ID:     id,
		Method: method,
		Params: rawParams,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("codex json-rpc: encode request: %w", err)
	}
	return data, nil
}

// EncodeNotification serializes a JSON-RPC notification (no id).
func EncodeNotification(method string, params any) ([]byte, error) {
	rawParams, err := encodeParams(params)
	if err != nil {
		return nil, err
	}
	msg := Message{
		Method: method,
		Params: rawParams,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("codex json-rpc: encode notification: %w", err)
	}
	return data, nil
}

// EncodeResponse serializes a JSON-RPC success response.
func EncodeResponse(id json.RawMessage, result any) ([]byte, error) {
	rawResult, err := encodeParams(result)
	if err != nil {
		return nil, err
	}
	msg := Message{
		ID:     id,
		Result: rawResult,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("codex json-rpc: encode response: %w", err)
	}
	return data, nil
}

// EncodeErrorResponse serializes a JSON-RPC error response.
func EncodeErrorResponse(id json.RawMessage, code int, message string) ([]byte, error) {
	msg := Message{
		ID: id,
		Error: &RPCError{
			Code:    code,
			Message: message,
		},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("codex json-rpc: encode error response: %w", err)
	}
	return data, nil
}

// encodeParams is a helper that JSON-encodes params or result values. nil is
// encoded as "null" which is valid JSON-RPC for absent params.
func encodeParams(v any) (json.RawMessage, error) {
	if v == nil {
		return json.RawMessage("null"), nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("codex json-rpc: marshal params: %w", err)
	}
	return json.RawMessage(data), nil
}

// ---------------------------------------------------------------------------
// Initialize handshake
//
// The codex app-server uses an LSP-like initialize handshake on connection
// establishment:
//  1. Client sends initialize request  →  {"id":1,"method":"initialize","params":{...}}
//  2. Server responds with capabilities →  {"id":1,"result":{...}}
//  3. Client sends initialized notification  →  {"method":"initialized","params":{}}
//
// All subsequent messages assume the server has been initialized.
// ---------------------------------------------------------------------------

// Well-known method names for the app-server initialize handshake.
const (
	MethodInitialize  = "initialize"
	MethodInitialized = "initialized"
)

// InitializeParams represents the parameters sent in an initialize request.
type InitializeParams struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapabilities `json:"capabilities"`
}

// ClientCapabilities describes the client-side capabilities advertised to the
// app-server during initialization.
type ClientCapabilities struct{}

// InitializeResult represents the expected response to an initialize request.
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      *ServerInfo        `json:"serverInfo,omitempty"`
}

// ServerCapabilities describes the server-side capabilities returned in the
// initialize response.
type ServerCapabilities struct{}

// ServerInfo carries identifying information about the app-server build.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ---------------------------------------------------------------------------
// Response shape validation
// ---------------------------------------------------------------------------

// Sentinel errors for JSON-RPC structural validation. Callers can use
// errors.Is to distinguish protocol-level failures from transport errors.
var (
	ErrInvalidMessage  = errors.New("codex json-rpc: invalid message")
	ErrInvalidResponse = errors.New("codex json-rpc: invalid response")
)

// isValidID reports whether raw is a valid JSON-RPC id: a JSON string or number.
// Object, array, boolean, and null are rejected.
func isValidID(raw json.RawMessage) bool {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return false
	}
	switch v.(type) {
	case string, float64:
		return true
	default:
		return false
	}
}

// VerifyResponse checks that msg is a structurally valid JSON-RPC response.
// It validates:
//   - id is present, non-null, and a scalar (string or number)
//   - exactly one of result or error is set
//   - method is empty (responses must not carry a method field)
//
// When expectedID is non-empty, the response id must match exactly.
func VerifyResponse(msg Message, expectedID json.RawMessage) error {
	if len(msg.ID) == 0 || string(msg.ID) == "null" {
		return fmt.Errorf("%w: response missing id", ErrInvalidMessage)
	}
	if !isValidID(msg.ID) {
		return fmt.Errorf("%w: response id must be a string or number", ErrInvalidMessage)
	}
	if msg.Method != "" {
		return fmt.Errorf("%w: response has unexpected method %q", ErrInvalidMessage, msg.Method)
	}
	hasResult := len(msg.Result) > 0
	hasError := msg.Error != nil
	if hasResult && hasError {
		return fmt.Errorf("%w: response has both result and error", ErrInvalidMessage)
	}
	if !hasResult && !hasError {
		return fmt.Errorf("%w: response has neither result nor error", ErrInvalidMessage)
	}
	if len(expectedID) > 0 && string(msg.ID) != string(expectedID) {
		return fmt.Errorf("%w: expected response id %s, got %s", ErrInvalidMessage, string(expectedID), string(msg.ID))
	}
	return nil
}

// DecodeResult verifies that msg is a valid success response and decodes its
// result field into target (which must be a non-nil pointer). When the
// response carries an error, DecodeResult returns ErrInvalidResponse wrapping
// the error details.
func DecodeResult(msg Message, target any) error {
	if err := VerifyResponse(msg, nil); err != nil {
		return err
	}
	if msg.Error != nil {
		return fmt.Errorf("%w: expected result, got error %d: %s", ErrInvalidResponse, msg.Error.Code, msg.Error.Message)
	}
	if err := json.Unmarshal(msg.Result, target); err != nil {
		return fmt.Errorf("codex json-rpc: decode result: %w", err)
	}
	return nil
}

// DecodeError verifies that msg is a valid error response and returns the
// contained *RPCError. Returns ErrInvalidResponse when the response is a
// success or structurally invalid.
func DecodeError(msg Message) (*RPCError, error) {
	if err := VerifyResponse(msg, nil); err != nil {
		return nil, err
	}
	if msg.Error == nil {
		return nil, fmt.Errorf("%w: expected error, got result", ErrInvalidResponse)
	}
	return msg.Error, nil
}

// IsWellFormedResponse checks basic structural validity of a response:
// non-empty id, exactly one of result or error set, and no method field.
func IsWellFormedResponse(msg Message) bool {
	return VerifyResponse(msg, nil) == nil
}

// IsWellFormedRequest checks basic structural validity of a request:
// a valid id (non-null string or number), non-empty method, and neither
// result nor error set.
func IsWellFormedRequest(msg Message) bool {
	if len(msg.ID) == 0 || string(msg.ID) == "null" || !isValidID(msg.ID) {
		return false
	}
	if msg.Method == "" {
		return false
	}
	if len(msg.Result) > 0 || msg.Error != nil {
		return false
	}
	return true
}

// ValidateMessage validates the JSON-RPC 2.0 structural constraints of raw
// message data beyond basic JSON syntax. It checks:
//   - data is valid JSON producing a Message struct
//   - id presence rules (present for request/response, absent for notification)
//   - mutual exclusion of result and error in responses
//   - method presence rules
//
// Returns nil for structurally valid messages.
func ValidateMessage(data []byte) error {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidMessage, err)
	}
	switch ClassifyMessage(msg) {
	case MsgRequest:
		if msg.Method == "" {
			return fmt.Errorf("%w: request missing method", ErrInvalidMessage)
		}
		if !isValidID(msg.ID) {
			return fmt.Errorf("%w: request id must be a string or number", ErrInvalidMessage)
		}
		if len(msg.Result) > 0 {
			return fmt.Errorf("%w: request must not carry result", ErrInvalidMessage)
		}
		if msg.Error != nil {
			return fmt.Errorf("%w: request must not carry error", ErrInvalidMessage)
		}
	case MsgNotification:
		if msg.Method == "" {
			return fmt.Errorf("%w: notification missing method", ErrInvalidMessage)
		}
		if len(msg.Result) > 0 {
			return fmt.Errorf("%w: notification must not carry result", ErrInvalidMessage)
		}
		if msg.Error != nil {
			return fmt.Errorf("%w: notification must not carry error", ErrInvalidMessage)
		}
	case MsgInvalid:
		return fmt.Errorf("%w: id must not be null when method is present", ErrInvalidMessage)
	case MsgResponse:
		if len(msg.ID) == 0 || string(msg.ID) == "null" {
			return fmt.Errorf("%w: response missing id", ErrInvalidMessage)
		}
		if msg.Error != nil && len(msg.Result) > 0 {
			return fmt.Errorf("%w: response has both result and error", ErrInvalidMessage)
		}
		if msg.Error == nil && len(msg.Result) == 0 {
			return fmt.Errorf("%w: response has neither result nor error", ErrInvalidMessage)
		}
		if msg.Method != "" {
			return fmt.Errorf("%w: response must not carry method", ErrInvalidMessage)
		}
	default:
		return fmt.Errorf("%w: unclassifiable message", ErrInvalidMessage)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Error classification helpers
// ---------------------------------------------------------------------------

// ErrorClass describes the retryability and severity of a JSON-RPC error.
type ErrorClass int

const (
	// ErrClassUnknown is the default classification.
	ErrClassUnknown ErrorClass = iota
	// ErrClassRetryable means the error may succeed on retry (e.g. overloaded,
	// transient network).
	ErrClassRetryable
	// ErrClassTerminal means the error will not succeed on retry (e.g. invalid
	// request, auth failure).
	ErrClassTerminal
	// ErrClassUnsupported means the requested method or capability is not
	// available on this server. Distinct from terminal errors because a
	// different server version or capability set may support it.
	ErrClassUnsupported
	// ErrClassProtocol means the error violates the JSON-RPC spec or the
	// app-server protocol handshake (e.g. not initialized).
	ErrClassProtocol
)

// String returns a human-readable label for the error class.
func (c ErrorClass) String() string {
	switch c {
	case ErrClassRetryable:
		return "retryable"
	case ErrClassTerminal:
		return "terminal"
	case ErrClassUnsupported:
		return "unsupported"
	case ErrClassProtocol:
		return "protocol"
	default:
		return "unknown"
	}
}

// ClassifyRPCError maps a JSON-RPC error code to an ErrorClass.
// - Overloaded (-32001) → ErrClassRetryable
// - Standard 4xx/5xx server errors → ErrClassRetryable
// - Standard parse/invalid/method/params errors → ErrClassTerminal
// - Not initialized / Already initialized → ErrClassProtocol
// - Internal → ErrClassRetryable (could be transient)
// - Everything else → ErrClassUnknown
func ClassifyRPCError(err *RPCError) ErrorClass {
	if err == nil {
		return ErrClassUnknown
	}
	switch err.Code {
	case ErrCodeOverloaded:
		return ErrClassRetryable
	case ErrCodeNotInitialized, ErrCodeAlreadyInitialized:
		return ErrClassProtocol
	case ErrCodeParseError, ErrCodeInvalidRequest, ErrCodeInvalidParams:
		return ErrClassTerminal
	case ErrCodeMethodNotFound:
		return ErrClassUnsupported
	case ErrCodeInternalError:
		return ErrClassRetryable
	default:
		// Server-side errors (positive or -3xx range) might be transient.
		// We classify conservatively.
		if err.Code <= -32000 && err.Code >= -32099 {
			return ErrClassRetryable
		}
		if err.Code <= -300 && err.Code >= -320 {
			return ErrClassRetryable
		}
		return ErrClassUnknown
	}
}

// IsRetryableError returns true when the error class is ErrClassRetryable.
func IsRetryableError(err *RPCError) bool {
	return ClassifyRPCError(err) == ErrClassRetryable
}

// IsProtocolError returns true when the error class is ErrClassProtocol.
func IsProtocolError(err *RPCError) bool {
	return ClassifyRPCError(err) == ErrClassProtocol
}

// IsUnsupportedError returns true when the error class is ErrClassUnsupported.
func IsUnsupportedError(err *RPCError) bool {
	return ClassifyRPCError(err) == ErrClassUnsupported
}

// IsOverloadedError returns true when the error is an overloaded (-32001)
// response.
func IsOverloadedError(err *RPCError) bool {
	return err != nil && err.Code == ErrCodeOverloaded
}

// RPCErrorFromError attempts to extract a *RPCError from an error chain via
// errors.As. Returns nil when the error is not a *RPCError.
func RPCErrorFromError(err error) *RPCError {
	if err == nil {
		return nil
	}
	switch e := err.(type) {
	case *RPCError:
		return e
	case interface{ Unwrap() error }:
		return RPCErrorFromError(e.Unwrap())
	default:
		return nil
	}
}

// ---------------------------------------------------------------------------
// Handshake state machine
//
// Encodes the required client-side initialize handshake sequence before
// regular requests are allowed:
//
//	PhaseInitial
//	  → OnInitializeSent() → PhaseInitializeSent
//	  → OnInitializeResponse() → PhaseInitialized
//	  → OnInitializedSent() → PhaseReady   ← regular requests allowed now
//
// Zero-value initialization is safe (starts in PhaseInitial).
// ---------------------------------------------------------------------------

// HandshakePhase describes the current state of the initialize handshake.
type HandshakePhase int

const (
	// PhaseInitial is before any handshake activity.
	PhaseInitial HandshakePhase = iota
	// PhaseInitializeSent after the initialize request has been sent but
	// before the response is received.
	PhaseInitializeSent
	// PhaseInitialized after the initialize response has been received and
	// validated; the initialized notification may now be sent.
	PhaseInitialized
	// PhaseReady after the initialized notification has been sent; regular
	// requests are now permitted.
	PhaseReady
)

// String returns a human-readable label for the handshake phase.
func (p HandshakePhase) String() string {
	switch p {
	case PhaseInitial:
		return "initial"
	case PhaseInitializeSent:
		return "initialize-sent"
	case PhaseInitialized:
		return "initialized"
	case PhaseReady:
		return "ready"
	default:
		return fmt.Sprintf("handshake-phase(%d)", int(p))
	}
}

// Handshake is a pure state machine that tracks the client-side initialize
// handshake sequence. It is safe for zero-value initialization (starts in
// PhaseInitial).
type Handshake struct {
	phase HandshakePhase
}

// Phase returns the current handshake phase.
func (h *Handshake) Phase() HandshakePhase {
	if h == nil {
		return PhaseInitial
	}
	return h.phase
}

// CanSendInitialize returns true in PhaseInitial, the only phase where
// sending an initialize request is valid.
func (h *Handshake) CanSendInitialize() bool {
	return h != nil && h.phase == PhaseInitial
}

// CanSendInitialized returns true in PhaseInitialized, the only phase where
// sending the initialized notification is valid.
func (h *Handshake) CanSendInitialized() bool {
	return h != nil && h.phase == PhaseInitialized
}

// CanSendRequest returns true in PhaseReady, meaning regular (non-handshake)
// requests are allowed.
func (h *Handshake) CanSendRequest() bool {
	return h != nil && h.phase == PhaseReady
}

// OnInitializeSent transitions from PhaseInitial to PhaseInitializeSent.
// Returns an error when the transition is invalid.
func (h *Handshake) OnInitializeSent() error {
	if h == nil {
		return errors.New("codex json-rpc: nil handshake")
	}
	if h.phase != PhaseInitial {
		return fmt.Errorf("codex json-rpc: cannot send initialize in phase %s", h.phase)
	}
	h.phase = PhaseInitializeSent
	return nil
}

// OnInitializeResponse transitions from PhaseInitializeSent to
// PhaseInitialized. Returns an error when the transition is invalid.
func (h *Handshake) OnInitializeResponse() error {
	if h == nil {
		return errors.New("codex json-rpc: nil handshake")
	}
	if h.phase != PhaseInitializeSent {
		return fmt.Errorf("codex json-rpc: unexpected initialize response in phase %s", h.phase)
	}
	h.phase = PhaseInitialized
	return nil
}

// OnInitializedSent transitions from PhaseInitialized to PhaseReady.
// Returns an error when the transition is invalid.
func (h *Handshake) OnInitializedSent() error {
	if h == nil {
		return errors.New("codex json-rpc: nil handshake")
	}
	if h.phase != PhaseInitialized {
		return fmt.Errorf("codex json-rpc: cannot send initialized in phase %s", h.phase)
	}
	h.phase = PhaseReady
	return nil
}

// Reset returns the handshake to PhaseInitial so the sequence can be
// restarted (e.g. on reconnection).
func (h *Handshake) Reset() {
	if h != nil {
		h.phase = PhaseInitial
	}
}

// IsReady returns true when the handshake is complete and regular requests
// are allowed.
func (h *Handshake) IsReady() bool {
	return h.CanSendRequest()
}
