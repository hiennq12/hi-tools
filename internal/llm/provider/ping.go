package provider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"hi-tools/internal/hix"
)

// PingResult là kết quả một lần ping provider, lưu trong _ping.json.
type PingResult struct {
	At        time.Time `json:"at"`
	OK        bool      `json:"ok"`
	LatencyMS int64     `json:"latency_ms"`
	Kind      string    `json:"kind,omitempty"`
	Error     string    `json:"error,omitempty"`
}

func pingsPath(dir string) string {
	return filepath.Join(dir, "_ping.json")
}

// LoadPings đọc _ping.json; file thiếu hoặc hỏng đều coi như rỗng.
func LoadPings(dir string) map[string]PingResult {
	data, err := os.ReadFile(pingsPath(dir))
	if err != nil {
		return map[string]PingResult{}
	}
	var pings map[string]PingResult
	if err := json.Unmarshal(data, &pings); err != nil {
		return map[string]PingResult{}
	}
	if pings == nil {
		pings = map[string]PingResult{}
	}
	return pings
}

// SavePings gộp results vào _ping.json, giữ nguyên kết quả cũ của provider khác.
func SavePings(dir string, results map[string]PingResult) error {
	pings := LoadPings(dir)
	for name, r := range results {
		pings[name] = r
	}
	data, err := json.Marshal(pings)
	if err != nil {
		return err
	}
	return hix.WriteFileAtomic(pingsPath(dir), data, 0o600)
}
