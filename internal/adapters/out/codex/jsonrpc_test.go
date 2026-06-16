package codex

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func intID(n int) json.RawMessage {
	data, _ := json.Marshal(n)
	return json.RawMessage(data)
}

func strID(s string) json.RawMessage {
	data, _ := json.Marshal(s)
	return json.RawMessage(data)
}

func nullID() json.RawMessage {
	return json.RawMessage("null")
}

func objID() json.RawMessage {
	return json.RawMessage(`{"foo":"bar"}`)
}

func arrID() json.RawMessage {
	return json.RawMessage(`[1,2,3]`)
}

func boolID(b bool) json.RawMessage {
	if b {
		return json.RawMessage(`true`)
	}
	return json.RawMessage(`false`)
}

func TestDecodeMessageRequest(t *testing.T) {
	data := []byte(`{"id":1,"method":"thread/list","params":{}}`)
	msg, err := DecodeMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(msg.ID) != "1" {
		t.Fatalf("expected id 1, got %s", string(msg.ID))
	}
	if msg.Method != "thread/list" {
		t.Fatalf("expected method thread/list, got %s", msg.Method)
	}
}

func TestDecodeMessageNotification(t *testing.T) {
	data := []byte(`{"method":"thread/started","params":{"thread":{"id":"thr_123"}}}`)
	msg, err := DecodeMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msg.ID) != 0 {
		t.Fatalf("expected no id for notification, got %s", string(msg.ID))
	}
	if msg.Method != "thread/started" {
		t.Fatalf("expected method thread/started, got %s", msg.Method)
	}
}

func TestDecodeMessageResponse(t *testing.T) {
	data := []byte(`{"id":1,"result":{"data":["thr_123"]}}`)
	msg, err := DecodeMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(msg.ID) != "1" {
		t.Fatalf("expected id 1, got %s", string(msg.ID))
	}
	if msg.Method != "" {
		t.Fatalf("expected no method for response, got %s", msg.Method)
	}
}

func TestDecodeMessageError(t *testing.T) {
	data := []byte(`{"id":1,"error":{"code":-32001,"message":"Server overloaded; retry later."}}`)
	msg, err := DecodeMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Error == nil {
		t.Fatal("expected error object")
	}
	if msg.Error.Code != -32001 {
		t.Fatalf("expected code -32001, got %d", msg.Error.Code)
	}
	if msg.Error.Message != "Server overloaded; retry later." {
		t.Fatalf("unexpected message: %s", msg.Error.Message)
	}
}

func TestDecodeMessageMalformed(t *testing.T) {
	_, err := DecodeMessage([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestClassifyMessage(t *testing.T) {
	tests := []struct {
		name string
		msg  Message
		want MessageKind
	}{
		{
			name: "request",
			msg:  Message{ID: intID(1), Method: "thread/list"},
			want: MsgRequest,
		},
		{
			name: "request string id",
			msg:  Message{ID: strID("req-1"), Method: "thread/list"},
			want: MsgRequest,
		},
		{
			name: "notification",
			msg:  Message{Method: "thread/started"},
			want: MsgNotification,
		},
		{
			name: "response with result",
			msg:  Message{ID: intID(1), Result: json.RawMessage(`{}`)},
			want: MsgResponse,
		},
		{
			name: "response with error",
			msg:  Message{ID: intID(1), Error: &RPCError{Code: -32603}},
			want: MsgResponse,
		},
		{
			name: "null id with method is invalid",
			msg:  Message{ID: nullID(), Method: "thread/list"},
			want: MsgInvalid,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyMessage(tt.msg)
			if got != tt.want {
				t.Errorf("ClassifyMessage() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestEncodeRequest(t *testing.T) {
	data, err := EncodeRequest(intID(10), "thread/list", map[string]any{"cursor": nil})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if string(msg.ID) != "10" {
		t.Fatalf("expected id 10, got %s", string(msg.ID))
	}
	if msg.Method != "thread/list" {
		t.Fatalf("expected method thread/list, got %s", msg.Method)
	}
}

func TestEncodeRequestNilParams(t *testing.T) {
	data, err := EncodeRequest(strID("req-1"), "initialize", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if string(msg.ID) != `"req-1"` {
		t.Fatalf("expected id req-1, got %s", string(msg.ID))
	}
}

func TestEncodeNotification(t *testing.T) {
	data, err := EncodeNotification("initialized", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if msg.Method != "initialized" {
		t.Fatalf("expected method initialized, got %s", msg.Method)
	}
	if len(msg.ID) != 0 {
		t.Fatalf("expected no id in notification, got %s", string(msg.ID))
	}
}

func TestEncodeResponse(t *testing.T) {
	result := map[string]string{"status": "ok"}
	data, err := EncodeResponse(intID(1), result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if string(msg.ID) != "1" {
		t.Fatalf("expected id 1, got %s", string(msg.ID))
	}
	var decoded map[string]string
	if err := json.Unmarshal(msg.Result, &decoded); err != nil {
		t.Fatalf("unmarshal result error: %v", err)
	}
	if decoded["status"] != "ok" {
		t.Fatalf("expected status ok, got %s", decoded["status"])
	}
}

func TestEncodeErrorResponse(t *testing.T) {
	data, err := EncodeErrorResponse(intID(1), -32001, "Server overloaded; retry later.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if msg.Error == nil {
		t.Fatal("expected error")
	}
	if msg.Error.Code != -32001 {
		t.Fatalf("expected code -32001, got %d", msg.Error.Code)
	}
}

func TestClassifyRPCError(t *testing.T) {
	tests := []struct {
		name  string
		code  int
		class ErrorClass
	}{
		{"overloaded", -32001, ErrClassRetryable},
		{"not initialized", -32002, ErrClassProtocol},
		{"already initialized", -32003, ErrClassProtocol},
		{"parse error", -32700, ErrClassTerminal},
		{"invalid request", -32600, ErrClassTerminal},
		{"method not found", -32601, ErrClassUnsupported},
		{"invalid params", -32602, ErrClassTerminal},
		{"internal error", -32603, ErrClassRetryable},
		{"custom server error", -32099, ErrClassRetryable},
		{"negative server range", -310, ErrClassRetryable},
		{"unknown positive", 1, ErrClassUnknown},
		{"nil error", 0, ErrClassUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rpcErr *RPCError
			if tt.code != 0 {
				rpcErr = &RPCError{Code: tt.code, Message: "test"}
			}
			got := ClassifyRPCError(rpcErr)
			if got != tt.class {
				t.Errorf("ClassifyRPCError(code=%d) = %s, want %s", tt.code, got, tt.class)
			}
		})
	}
}

func TestClassifyRPCErrorNil(t *testing.T) {
	if got := ClassifyRPCError(nil); got != ErrClassUnknown {
		t.Fatalf("expected unknown for nil, got %s", got)
	}
}

func TestIsRetryableError(t *testing.T) {
	if !IsRetryableError(&RPCError{Code: -32001}) {
		t.Fatal("expected overloaded to be retryable")
	}
	if IsRetryableError(&RPCError{Code: -32601}) {
		t.Fatal("expected method-not-found to be NOT retryable")
	}
	if IsRetryableError(nil) {
		t.Fatal("expected nil to be NOT retryable")
	}
}

func TestIsProtocolError(t *testing.T) {
	if !IsProtocolError(&RPCError{Code: -32002}) {
		t.Fatal("expected not-initialized to be protocol")
	}
	if IsProtocolError(&RPCError{Code: -32001}) {
		t.Fatal("expected overloaded to be NOT protocol")
	}
}

func TestIsUnsupportedError(t *testing.T) {
	if !IsUnsupportedError(&RPCError{Code: -32601}) {
		t.Fatal("expected method-not-found to be unsupported")
	}
	if IsUnsupportedError(&RPCError{Code: -32600}) {
		t.Fatal("expected invalid-request to NOT be unsupported")
	}
	if IsUnsupportedError(nil) {
		t.Fatal("expected nil to NOT be unsupported")
	}
}

func TestIsOverloadedError(t *testing.T) {
	if !IsOverloadedError(&RPCError{Code: -32001}) {
		t.Fatal("expected -32001 to be overloaded")
	}
	if IsOverloadedError(&RPCError{Code: -32603}) {
		t.Fatal("expected -32603 to NOT be overloaded")
	}
	if IsOverloadedError(nil) {
		t.Fatal("expected nil to NOT be overloaded")
	}
}

type multiWrappedError struct {
	errs []error
}

func (e multiWrappedError) Error() string {
	return "multi wrapped error"
}

func (e multiWrappedError) Unwrap() []error {
	return e.errs
}

func TestRPCErrorFromError(t *testing.T) {
	rpcErr := &RPCError{Code: -32001, Message: "overloaded"}

	// Direct extraction should work on *RPCError.
	got := RPCErrorFromError(rpcErr)
	if got == nil || got.Code != -32001 {
		t.Fatal("expected direct extraction to work")
	}

	// Wrapped error (via fmt.Errorf %%w) should still find it via Unwrap chain.
	wrappedWith := fmt.Errorf("outer: %w", rpcErr)
	got2 := RPCErrorFromError(wrappedWith)
	if got2 == nil || got2.Code != -32001 {
		t.Fatal("expected wrapped extraction to work")
	}

	joined := errors.Join(errors.New("plain"), rpcErr)
	got3 := RPCErrorFromError(joined)
	if got3 == nil || got3.Code != -32001 {
		t.Fatal("expected joined extraction to work")
	}

	customWrapped := multiWrappedError{errs: []error{errors.New("plain"), fmt.Errorf("inner: %w", rpcErr)}}
	got4 := RPCErrorFromError(customWrapped)
	if got4 == nil || got4.Code != -32001 {
		t.Fatal("expected custom wrapped extraction to work")
	}
}

func TestRPCErrorFromErrorNonRPC(t *testing.T) {
	if got := RPCErrorFromError(errors.New("plain error")); got != nil {
		t.Fatal("expected nil for non-RPC error")
	}
	if got := RPCErrorFromError(nil); got != nil {
		t.Fatal("expected nil for nil")
	}
}

func TestRPCErrorError(t *testing.T) {
	e := &RPCError{Code: -32001, Message: "Server overloaded; retry later."}
	want := "JSON-RPC error -32001: Server overloaded; retry later."
	if got := e.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}

	e2 := &RPCError{Code: -32601}
	if got := e2.Error(); got != "JSON-RPC error -32601" {
		t.Fatalf("Error() with no message = %q", got)
	}

	var nilErr *RPCError
	if got := nilErr.Error(); got != "" {
		t.Fatalf("expected empty for nil, got %q", got)
	}
}

func TestMessageKindString(t *testing.T) {
	tests := []struct {
		k    MessageKind
		want string
	}{
		{MsgRequest, "request"},
		{MsgNotification, "notification"},
		{MsgResponse, "response"},
		{MsgInvalid, "invalid"},
		{MessageKind(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.k.String(); got != tt.want {
			t.Errorf("MessageKind(%d).String() = %q, want %q", tt.k, got, tt.want)
		}
	}
}

func TestErrorClassString(t *testing.T) {
	tests := []struct {
		c    ErrorClass
		want string
	}{
		{ErrClassUnknown, "unknown"},
		{ErrClassRetryable, "retryable"},
		{ErrClassTerminal, "terminal"},
		{ErrClassUnsupported, "unsupported"},
		{ErrClassProtocol, "protocol"},
		{ErrorClass(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.c.String(); got != tt.want {
			t.Errorf("ErrorClass(%d).String() = %q, want %q", tt.c, got, tt.want)
		}
	}
}

func TestMethodConstants(t *testing.T) {
	if MethodInitialize != "initialize" {
		t.Fatalf("MethodInitialize = %q, want %q", MethodInitialize, "initialize")
	}
	if MethodInitialized != "initialized" {
		t.Fatalf("MethodInitialized = %q, want %q", MethodInitialized, "initialized")
	}
}

func TestInitializeParamsRoundTrip(t *testing.T) {
	params := InitializeParams{
		ProtocolVersion: "2.0",
		Capabilities:    ClientCapabilities{},
	}
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded InitializeParams
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.ProtocolVersion != "2.0" {
		t.Fatalf("ProtocolVersion = %q, want %q", decoded.ProtocolVersion, "2.0")
	}
}

func TestInitializeResultRoundTrip(t *testing.T) {
	result := InitializeResult{
		ProtocolVersion: "2.0",
		Capabilities:    ServerCapabilities{},
		ServerInfo: &ServerInfo{
			Name:    "codex",
			Version: "1.0.0",
		},
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded InitializeResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.ProtocolVersion != "2.0" {
		t.Fatalf("ProtocolVersion = %q", decoded.ProtocolVersion)
	}
	if decoded.ServerInfo == nil || decoded.ServerInfo.Name != "codex" {
		t.Fatalf("ServerInfo.Name = %v, want \"codex\"", decoded.ServerInfo)
	}
}

func TestVerifyResponseValid(t *testing.T) {
	msg := Message{ID: intID(1), Result: json.RawMessage(`{"ok":true}`)}
	if err := VerifyResponse(msg, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyResponseValidWithIDCheck(t *testing.T) {
	msg := Message{ID: intID(1), Result: json.RawMessage(`{}`)}
	if err := VerifyResponse(msg, intID(1)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyResponseMissingID(t *testing.T) {
	msg := Message{Result: json.RawMessage(`{}`)}
	if err := VerifyResponse(msg, nil); err == nil {
		t.Fatal("expected error for missing id")
	}
}

func TestVerifyResponseNullID(t *testing.T) {
	msg := Message{ID: nullID(), Result: json.RawMessage(`{}`)}
	if err := VerifyResponse(msg, nil); err == nil {
		t.Fatal("expected error for null id")
	}
}

func TestVerifyResponseWithMethod(t *testing.T) {
	msg := Message{ID: intID(1), Method: "initialize", Result: json.RawMessage(`{}`)}
	if err := VerifyResponse(msg, nil); err == nil {
		t.Fatal("expected error for response with method")
	}
}

func TestVerifyResponseBothResultAndError(t *testing.T) {
	msg := Message{
		ID:     intID(1),
		Result: json.RawMessage(`{}`),
		Error:  &RPCError{Code: -32603, Message: "internal"},
	}
	if err := VerifyResponse(msg, nil); err == nil {
		t.Fatal("expected error for both result and error")
	}
}

func TestVerifyResponseNeitherResultNorError(t *testing.T) {
	msg := Message{ID: intID(1)}
	if err := VerifyResponse(msg, nil); err == nil {
		t.Fatal("expected error for neither result nor error")
	}
}

func TestVerifyResponseIDMismatch(t *testing.T) {
	msg := Message{ID: intID(42), Result: json.RawMessage(`{}`)}
	if err := VerifyResponse(msg, intID(1)); err == nil {
		t.Fatal("expected error for id mismatch")
	}
}

func TestVerifyResponseIDMismatchWrapsSentinel(t *testing.T) {
	msg := Message{ID: intID(42), Result: json.RawMessage(`{}`)}
	if err := VerifyResponse(msg, intID(1)); !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("expected ErrInvalidMessage, got %v", err)
	}
}

func TestVerifyResponseInvalidIDObject(t *testing.T) {
	msg := Message{ID: objID(), Result: json.RawMessage(`{}`)}
	err := VerifyResponse(msg, nil)
	if err == nil {
		t.Fatal("expected error for object id")
	}
	if !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("expected ErrInvalidMessage, got %v", err)
	}
}

func TestVerifyResponseInvalidIDArray(t *testing.T) {
	msg := Message{ID: arrID(), Result: json.RawMessage(`{}`)}
	err := VerifyResponse(msg, nil)
	if err == nil {
		t.Fatal("expected error for array id")
	}
	if !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("expected ErrInvalidMessage, got %v", err)
	}
}

func TestVerifyResponseInvalidIDBool(t *testing.T) {
	msg := Message{ID: boolID(true), Result: json.RawMessage(`{}`)}
	err := VerifyResponse(msg, nil)
	if err == nil {
		t.Fatal("expected error for bool id")
	}
	if !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("expected ErrInvalidMessage, got %v", err)
	}
}

func TestDecodeResultSuccess(t *testing.T) {
	type hello struct {
		Greeting string `json:"greeting"`
	}
	msg := Message{ID: intID(1), Result: json.RawMessage(`{"greeting":"hello"}`)}
	var target hello
	if err := DecodeResult(msg, &target); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.Greeting != "hello" {
		t.Fatalf("greeting = %q, want %q", target.Greeting, "hello")
	}
}

func TestDecodeResultWithError(t *testing.T) {
	msg := Message{ID: intID(1), Error: &RPCError{Code: -32001, Message: "overloaded"}}
	var target any
	if err := DecodeResult(msg, &target); err == nil {
		t.Fatal("expected error for error response")
	}
}

func TestDecodeResultInvalidShape(t *testing.T) {
	msg := Message{ID: intID(1)}
	var target any
	if err := DecodeResult(msg, &target); err == nil {
		t.Fatal("expected error for invalid response shape")
	}
}

func TestDecodeErrorSuccess(t *testing.T) {
	msg := Message{ID: intID(1), Error: &RPCError{Code: -32603, Message: "internal error"}}
	rpcErr, err := DecodeError(msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rpcErr.Code != -32603 {
		t.Fatalf("code = %d, want %d", rpcErr.Code, -32603)
	}
	if rpcErr.Message != "internal error" {
		t.Fatalf("message = %q, want %q", rpcErr.Message, "internal error")
	}
}

func TestDecodeErrorWithResult(t *testing.T) {
	msg := Message{ID: intID(1), Result: json.RawMessage(`{}`)}
	_, err := DecodeError(msg)
	if err == nil {
		t.Fatal("expected error when response has result")
	}
}

func TestDecodeErrorInvalidShape(t *testing.T) {
	msg := Message{ID: intID(1)}
	_, err := DecodeError(msg)
	if err == nil {
		t.Fatal("expected error for invalid response shape")
	}
}

func TestIsWellFormedResponse(t *testing.T) {
	tests := []struct {
		name string
		msg  Message
		want bool
	}{
		{"valid result", Message{ID: intID(1), Result: json.RawMessage(`{}`)}, true},
		{"valid error", Message{ID: intID(1), Error: &RPCError{Code: -1}}, true},
		{"missing id", Message{Result: json.RawMessage(`{}`)}, false},
		{"null id", Message{ID: nullID(), Result: json.RawMessage(`{}`)}, false},
		{"both result and error", Message{ID: intID(1), Result: json.RawMessage(`{}`), Error: &RPCError{Code: -1}}, false},
		{"method present", Message{ID: intID(1), Method: "x", Result: json.RawMessage(`{}`)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsWellFormedResponse(tt.msg)
			if got != tt.want {
				t.Errorf("IsWellFormedResponse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsWellFormedRequest(t *testing.T) {
	tests := []struct {
		name string
		msg  Message
		want bool
	}{
		{"valid numeric id", Message{ID: intID(1), Method: "initialize"}, true},
		{"valid string id", Message{ID: strID("abc"), Method: "initialize"}, true},
		{"missing id", Message{Method: "initialize"}, false},
		{"null id", Message{ID: nullID(), Method: "x"}, false},
		{"object id", Message{ID: objID(), Method: "x"}, false},
		{"array id", Message{ID: arrID(), Method: "x"}, false},
		{"bool id true", Message{ID: boolID(true), Method: "x"}, false},
		{"bool id false", Message{ID: boolID(false), Method: "x"}, false},
		{"missing method", Message{ID: intID(1)}, false},
		{"with result", Message{ID: intID(1), Method: "x", Result: json.RawMessage(`{}`)}, false},
		{"with error", Message{ID: intID(1), Method: "x", Error: &RPCError{Code: -1}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsWellFormedRequest(tt.msg)
			if got != tt.want {
				t.Errorf("IsWellFormedRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateMessageValid(t *testing.T) {
	t.Run("request", func(t *testing.T) {
		if err := ValidateMessage([]byte(`{"id":1,"method":"initialize"}`)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("notification", func(t *testing.T) {
		if err := ValidateMessage([]byte(`{"method":"initialized"}`)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("response result", func(t *testing.T) {
		if err := ValidateMessage([]byte(`{"id":1,"result":{}}`)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("response error", func(t *testing.T) {
		if err := ValidateMessage([]byte(`{"id":1,"error":{"code":-32603,"message":"err"}}`)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestValidateMessageMalformedJSON(t *testing.T) {
	if err := ValidateMessage([]byte(`not json`)); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestValidateMessageBothResultAndError(t *testing.T) {
	data := []byte(`{"id":1,"result":{},"error":{"code":-1,"message":"x"}}`)
	if err := ValidateMessage(data); err == nil {
		t.Fatal("expected error for both result and error")
	}
}

func TestValidateMessageResponseNeitherResultNorError(t *testing.T) {
	data := []byte(`{"id":1}`)
	if err := ValidateMessage(data); err == nil {
		t.Fatal("expected error for response with neither")
	}
}

func TestValidateMessageResponseWithMethod(t *testing.T) {
	data := []byte(`{"id":1,"method":"x","result":{}}`)
	if err := ValidateMessage(data); err == nil {
		t.Fatal("expected error for response with method")
	}
}

func TestValidateMessageRequestWithResult(t *testing.T) {
	data := []byte(`{"id":1,"method":"x","result":{}}`)
	if err := ValidateMessage(data); err == nil {
		t.Fatal("expected error for request with result")
	}
}

func TestValidateMessageRequestWithError(t *testing.T) {
	data := []byte(`{"id":1,"method":"x","error":{"code":-1,"message":"x"}}`)
	if err := ValidateMessage(data); err == nil {
		t.Fatal("expected error for request with error")
	}
}

func TestValidateMessageRequestWithInvalidID(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"object id", []byte(`{"id":{"foo":"bar"},"method":"x"}`)},
		{"array id", []byte(`{"id":[1,2,3],"method":"x"}`)},
		{"bool id", []byte(`{"id":true,"method":"x"}`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMessage(tt.data)
			if err == nil {
				t.Fatal("expected error for request with invalid id shape")
			}
			if !errors.Is(err, ErrInvalidMessage) {
				t.Fatalf("expected ErrInvalidMessage, got %v", err)
			}
		})
	}
}

func TestValidateMessageNotificationWithResult(t *testing.T) {
	data := []byte(`{"method":"x","result":{}}`)
	if err := ValidateMessage(data); err == nil {
		t.Fatal("expected error for notification with result")
	}
}

func TestValidateMessageNotificationWithError(t *testing.T) {
	data := []byte(`{"method":"x","error":{"code":-1,"message":"x"}}`)
	if err := ValidateMessage(data); err == nil {
		t.Fatal("expected error for notification with error")
	}
}

func TestValidateMessageNullIDWithMethod(t *testing.T) {
	data := []byte(`{"id":null,"method":"initialize"}`)
	err := ValidateMessage(data)
	if err == nil {
		t.Fatal("expected error for null id with method")
	}
	if !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("expected ErrInvalidMessage, got %v", err)
	}
}

func TestValidateMessageSentinelErrors(t *testing.T) {
	err := ValidateMessage([]byte(`{"id":1}`))
	if !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("expected ErrInvalidMessage, got %v", err)
	}
	err = ValidateMessage([]byte(`not json`))
	if !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("expected ErrInvalidMessage for malformed JSON, got %v", err)
	}
}

func TestErrInvalidResponseWrapping(t *testing.T) {
	// Verify DecodeResult wraps ErrInvalidResponse for protocol mismatches.
	msg := Message{ID: intID(1), Error: &RPCError{Code: -32001, Message: "overloaded"}}
	var target any
	err := DecodeResult(msg, &target)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("expected ErrInvalidResponse, got %v", err)
	}
}

func TestHandshakePhaseString(t *testing.T) {
	tests := []struct {
		p    HandshakePhase
		want string
	}{
		{PhaseInitial, "initial"},
		{PhaseInitializeSent, "initialize-sent"},
		{PhaseInitialized, "initialized"},
		{PhaseReady, "ready"},
		{HandshakePhase(99), "handshake-phase(99)"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.p.String(); got != tt.want {
				t.Errorf("HandshakePhase.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHandshakeZeroValueIsInitial(t *testing.T) {
	var h Handshake
	if got := h.Phase(); got != PhaseInitial {
		t.Fatalf("zero-value phase = %s, want %s", got, PhaseInitial)
	}
}

func TestHandshakeFullSequence(t *testing.T) {
	var h Handshake

	if !h.CanSendInitialize() {
		t.Fatal("expected CanSendInitialize true at start")
	}
	if h.CanSendInitialized() {
		t.Fatal("expected CanSendInitialized false at start")
	}
	if h.CanSendRequest() {
		t.Fatal("expected CanSendRequest false at start")
	}
	if h.IsReady() {
		t.Fatal("expected IsReady false at start")
	}

	// Step 1: send initialize request.
	if err := h.OnInitializeSent(); err != nil {
		t.Fatalf("OnInitializeSent: %v", err)
	}
	if h.Phase() != PhaseInitializeSent {
		t.Fatalf("phase = %s, want %s", h.Phase(), PhaseInitializeSent)
	}
	if h.CanSendInitialize() {
		t.Fatal("CanSendInitialize should be false after sending initialize")
	}
	if h.CanSendRequest() {
		t.Fatal("CanSendRequest should be false before ready")
	}

	// Step 2: receive initialize response.
	if err := h.OnInitializeResponse(); err != nil {
		t.Fatalf("OnInitializeResponse: %v", err)
	}
	if h.Phase() != PhaseInitialized {
		t.Fatalf("phase = %s, want %s", h.Phase(), PhaseInitialized)
	}
	if !h.CanSendInitialized() {
		t.Fatal("CanSendInitialized should be true after response")
	}
	if h.CanSendRequest() {
		t.Fatal("CanSendRequest should still be false before notification sent")
	}

	// Step 3: send initialized notification.
	if err := h.OnInitializedSent(); err != nil {
		t.Fatalf("OnInitializedSent: %v", err)
	}
	if h.Phase() != PhaseReady {
		t.Fatalf("phase = %s, want %s", h.Phase(), PhaseReady)
	}
	if !h.CanSendRequest() {
		t.Fatal("CanSendRequest should be true in ready phase")
	}
	if !h.IsReady() {
		t.Fatal("IsReady should be true in ready phase")
	}
}

func TestHandshakeInvalidTransitions(t *testing.T) {
	// Response before initialize.
	var h1 Handshake
	if err := h1.OnInitializeResponse(); err == nil {
		t.Error("OnInitializeResponse before OnInitializeSent should error")
	}
	if h1.Phase() != PhaseInitial {
		t.Error("phase should remain initial after failed OnInitializeResponse")
	}

	// Initialize notification before initialize.
	var h2 Handshake
	if err := h2.OnInitializedSent(); err == nil {
		t.Error("OnInitializedSent before OnInitializeSent should error")
	}
	if h2.Phase() != PhaseInitial {
		t.Error("phase should remain initial after failed OnInitializedSent")
	}

	// Send initialize twice.
	var h3 Handshake
	_ = h3.OnInitializeSent()
	if err := h3.OnInitializeSent(); err == nil {
		t.Error("second OnInitializeSent should error")
	}
	if h3.Phase() != PhaseInitializeSent {
		t.Error("phase should remain initialize-sent after failed second OnInitializeSent")
	}

	// Initialize notification before response.
	var h4 Handshake
	_ = h4.OnInitializeSent()
	if err := h4.OnInitializedSent(); err == nil {
		t.Error("OnInitializedSent before OnInitializeResponse should error")
	}
	if h4.Phase() != PhaseInitializeSent {
		t.Error("phase should remain initialize-sent after failed OnInitializedSent")
	}

	// Response at initialized phase (double response).
	var h5 Handshake
	_ = h5.OnInitializeSent()
	_ = h5.OnInitializeResponse()
	if err := h5.OnInitializeResponse(); err == nil {
		t.Error("second OnInitializeResponse should error")
	}
	if h5.Phase() != PhaseInitialized {
		t.Error("phase should remain initialized after failed second OnInitializeResponse")
	}
}

func TestHandshakeReset(t *testing.T) {
	var h Handshake
	_ = h.OnInitializeSent()
	_ = h.OnInitializeResponse()
	_ = h.OnInitializedSent()
	if !h.IsReady() {
		t.Fatal("expected ready before reset")
	}

	h.Reset()
	if h.Phase() != PhaseInitial {
		t.Fatalf("after reset phase = %s, want %s", h.Phase(), PhaseInitial)
	}
	if !h.CanSendInitialize() {
		t.Fatal("CanSendInitialize should be true after reset")
	}
}

func TestHandshakeNilSafety(t *testing.T) {
	var nilH *Handshake
	if got := nilH.Phase(); got != PhaseInitial {
		t.Fatalf("nil Phase() = %s, want %s", got, PhaseInitial)
	}
	if nilH.CanSendInitialize() {
		t.Fatal("nil CanSendInitialize should be false")
	}
	if nilH.CanSendInitialized() {
		t.Fatal("nil CanSendInitialized should be false")
	}
	if nilH.CanSendRequest() {
		t.Fatal("nil CanSendRequest should be false")
	}
	if nilH.IsReady() {
		t.Fatal("nil IsReady should be false")
	}
	if err := nilH.OnInitializeSent(); err == nil {
		t.Fatal("nil OnInitializeSent should error")
	}
	if err := nilH.OnInitializeResponse(); err == nil {
		t.Fatal("nil OnInitializeResponse should error")
	}
	if err := nilH.OnInitializedSent(); err == nil {
		t.Fatal("nil OnInitializedSent should error")
	}
	// Reset on nil should not panic.
	nilH.Reset()
}

func TestSentinelErrorsAreDistinct(t *testing.T) {
	if ErrInvalidMessage == ErrInvalidResponse {
		t.Fatal("ErrInvalidMessage and ErrInvalidResponse must be distinct")
	}
}
