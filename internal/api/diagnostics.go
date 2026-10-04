package api

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"regexp"
	"slices"
)

// Only structured, bounded fields: never accept raw SDK logs or arbitrary error strings.
type diagnosticEvent struct {
	ID      string             `json:"id"`
	Call    string             `json:"call"`
	Kind    string             `json:"kind"`
	Version string             `json:"version"`
	OS      string             `json:"os"`
	Arch    string             `json:"arch"`
	Codec   string             `json:"codec,omitempty"`
	Encoder string             `json:"encoder,omitempty"`
	Stage   string             `json:"stage,omitempty"`
	Code    string             `json:"code,omitempty"`
	Metrics map[string]float64 `json:"metrics,omitempty"`
}

var diagnosticID = regexp.MustCompile(`^[a-f0-9]{32}$`)
var diagnosticVersion = regexp.MustCompile(`^[a-zA-Z0-9.+_-]{1,64}$`)
var diagnosticMetrics = []string{"duration_s", "errors", "suppressed", "fallbacks", "events_dropped", "samples", "video_fps_avg", "video_fps_min", "video_frames", "video_dropped", "video_freezes", "audio_concealed", "packets_lost", "bytes_sent", "bytes_received", "jitter_ms_max", "width", "height", "fps_limit", "bitrate_limit", "av1", "vp9", "h264"}

func (e diagnosticEvent) valid() bool {
	if !diagnosticID.MatchString(e.ID) || !diagnosticID.MatchString(e.Call) || !diagnosticVersion.MatchString(e.Version) {
		return false
	}
	if !slices.Contains([]string{"macos", "windows", "linux"}, e.OS) || !slices.Contains([]string{"aarch64", "x86_64", "x86"}, e.Arch) {
		return false
	}
	if !slices.Contains([]string{"call_start", "call_connected", "call_end", "screen_started", "screen_stopped", "codec_changed", "encoder_fallback", "error", "reconnecting", "reconnected"}, e.Kind) {
		return false
	}
	if !slices.Contains([]string{"", "av1", "vp9", "h264"}, e.Codec) || !slices.Contains([]string{"", "nvenc", "amf", "software", "sdk_auto"}, e.Encoder) {
		return false
	}
	if !slices.Contains([]string{"", "connect", "audio", "video", "subscribe", "network", "stats"}, e.Stage) || !slices.Contains([]string{"", "permission", "timeout", "device", "network", "codec", "unknown", "gpu_failed"}, e.Code) {
		return false
	}
	if len(e.Metrics) > len(diagnosticMetrics) {
		return false
	}
	for k, v := range e.Metrics {
		if !slices.Contains(diagnosticMetrics, k) || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1e15 {
			return false
		}
	}
	return true
}
func (s *Server) desktopDiagnostic(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var event diagnosticEvent
	if err := dec.Decode(&event); err != nil || !event.valid() {
		writeError(w, 400, "Invalid diagnostic report")
		return
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		writeError(w, 400, "Invalid diagnostic report")
		return
	}
	user := currentUser(r)
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 500, "Could not store report")
		return
	}
	defer tx.Rollback()
	// Serialize uploads per user, including the hourly limit and duplicate checks.
	var id string
	if err = tx.QueryRowContext(r.Context(), "SELECT id FROM users WHERE id=$1 FOR UPDATE", user.ID).Scan(&id); err != nil {
		writeError(w, 500, "Could not store report")
		return
	}
	var exists bool
	if err = tx.QueryRowContext(r.Context(), "SELECT EXISTS(SELECT 1 FROM desktop_diagnostics WHERE user_id=$1 AND event_id=$2)", user.ID, event.ID).Scan(&exists); err != nil {
		writeError(w, 500, "Could not store report")
		return
	}
	if exists {
		w.WriteHeader(204)
		return
	}
	var count int
	if err = tx.QueryRowContext(r.Context(), "SELECT count(*) FROM desktop_diagnostics WHERE user_id=$1 AND received_at>now()-interval '1 hour'", user.ID).Scan(&count); err != nil {
		writeError(w, 500, "Could not store report")
		return
	}
	if count >= 120 {
		w.Header().Set("Retry-After", "3600")
		writeError(w, 429, "Diagnostic limit reached")
		return
	}
	body, _ := json.Marshal(event)
	_, err = tx.ExecContext(r.Context(), "INSERT INTO desktop_diagnostics(user_id,event_id,call_id,payload) VALUES($1,$2,$3,$4)", user.ID, event.ID, event.Call, string(body))
	if err != nil {
		writeError(w, 500, "Could not store report")
		return
	}
	if err = tx.Commit(); err != nil {
		writeError(w, 500, "Could not store report")
		return
	}
	w.WriteHeader(204)
}
