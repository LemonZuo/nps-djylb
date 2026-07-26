package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/djylb/nps/lib/logs"
)

// Envelope is the shape every JSON response takes, successful or not. Keeping
// a single shape means the frontend has exactly one place to unwrap results
// and one place to surface errors.
type Envelope struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Data      any    `json:"data,omitempty"`
	RequestID string `json:"requestId,omitempty"`
}

// List is the payload shape for every paginated collection endpoint.
type List struct {
	Rows  any   `json:"rows"`
	Total int64 `json:"total"`
}

// Application error codes. These are deliberately coarse: the HTTP status
// carries the category and the code lets the frontend branch on the few cases
// where it must react differently (re-login, re-solve a captcha, etc).
const (
	CodeOK           = 0
	CodeBadRequest   = 40000
	CodeUnauthorized = 40100 // no/invalid credentials -> frontend must re-login
	CodeTokenExpired = 40101 // valid signature, past exp -> same, but distinguishable
	CodeForbidden    = 40300
	CodeNotFound     = 40400
	CodeConflict     = 40900
	CodeTooManyReqs  = 42900 // rate-limited or temporarily banned
	CodeInternal     = 50000
)

// requestIDKey is the context key under which the middleware stores the id.
type ctxKey int

const requestIDKey ctxKey = iota

// newRequestID returns a short random hex id used to correlate a client-visible
// error with the server log line that explains it.
func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing is not recoverable in any useful way here, and a
		// missing correlation id must never fail an otherwise good request.
		return ""
	}
	return hex.EncodeToString(b[:])
}

// RequestID returns the id assigned to r, or "" if the request did not pass
// through the middleware.
func RequestID(r *http.Request) string {
	id, _ := r.Context().Value(requestIDKey).(string)
	return id
}

// writeJSON serialises v with the given status. It marshals before touching the
// header so that an encoding failure cannot emit a 200 with a truncated body.
func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		logs.Error("api: marshal response: %v", err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":50000,"message":"internal error"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// Ok writes a 200 with the given payload.
func Ok(w http.ResponseWriter, r *http.Request, data any) {
	writeJSON(w, http.StatusOK, Envelope{
		Code:      CodeOK,
		Message:   "ok",
		Data:      data,
		RequestID: RequestID(r),
	})
}

// OkList writes a 200 wrapping a collection and its unpaginated total.
func OkList(w http.ResponseWriter, r *http.Request, rows any, total int64) {
	Ok(w, r, List{Rows: rows, Total: total})
}

// Fail writes an error response. status is the HTTP status; code is the
// application code from the list above; message is shown to the user, so it
// must not leak internal detail.
func Fail(w http.ResponseWriter, r *http.Request, status, code int, message string) {
	writeJSON(w, status, Envelope{
		Code:      code,
		Message:   message,
		RequestID: RequestID(r),
	})
}

// The helpers below cover the cases used throughout the handlers, so that the
// status and the application code can never drift apart by accident.

func BadRequest(w http.ResponseWriter, r *http.Request, message string) {
	Fail(w, r, http.StatusBadRequest, CodeBadRequest, message)
}

func Unauthorized(w http.ResponseWriter, r *http.Request, message string) {
	Fail(w, r, http.StatusUnauthorized, CodeUnauthorized, message)
}

func TokenExpired(w http.ResponseWriter, r *http.Request, message string) {
	Fail(w, r, http.StatusUnauthorized, CodeTokenExpired, message)
}

func Forbidden(w http.ResponseWriter, r *http.Request, message string) {
	Fail(w, r, http.StatusForbidden, CodeForbidden, message)
}

func NotFound(w http.ResponseWriter, r *http.Request, message string) {
	Fail(w, r, http.StatusNotFound, CodeNotFound, message)
}

func Conflict(w http.ResponseWriter, r *http.Request, message string) {
	Fail(w, r, http.StatusConflict, CodeConflict, message)
}

func TooManyRequests(w http.ResponseWriter, r *http.Request, message string) {
	Fail(w, r, http.StatusTooManyRequests, CodeTooManyReqs, message)
}

// Internal logs the underlying cause against the request id and returns a
// generic message, so that stack detail never reaches the client.
func Internal(w http.ResponseWriter, r *http.Request, cause error) {
	logs.Error("api: request %s: %v", RequestID(r), cause)
	Fail(w, r, http.StatusInternalServerError, CodeInternal, "internal error")
}
