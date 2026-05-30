package tunnel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type BlockedIPState struct {
	IP           string `json:"ip"`
	BlockedUntil string `json:"blocked_until"`
	RemainingSec int64  `json:"remaining_sec"`
}

type persistedRuntimeState struct {
	BlockedIPs []persistedBlockedIP `json:"blocked_ips"`
}

type persistedBlockedIP struct {
	IP           string    `json:"ip"`
	BlockedUntil time.Time `json:"blocked_until"`
}

func LoadBlockedIPs(stateFile string) ([]BlockedIPState, error) {
	stateFile = strings.TrimSpace(stateFile)
	if stateFile == "" {
		return nil, nil
	}
	stateFile = filepath.Clean(stateFile)
	if stateFile == "." {
		return nil, nil
	}
	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var state persistedRuntimeState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return blockedIPStates(state.BlockedIPs, time.Now()), nil
}

func UnblockBlockedIP(stateFile, ip string) (bool, error) {
	stateFile = strings.TrimSpace(stateFile)
	ip = strings.TrimSpace(ip)
	if stateFile == "" || ip == "" {
		return false, nil
	}
	stateFile = filepath.Clean(stateFile)
	if stateFile == "." {
		return false, nil
	}
	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	var state persistedRuntimeState
	if err := json.Unmarshal(data, &state); err != nil {
		return false, err
	}
	now := time.Now()
	removed := false
	kept := state.BlockedIPs[:0]
	for _, item := range state.BlockedIPs {
		if item.IP == ip {
			removed = true
			continue
		}
		if item.IP == "" || item.BlockedUntil.IsZero() || !item.BlockedUntil.After(now) {
			continue
		}
		kept = append(kept, item)
	}
	state.BlockedIPs = kept
	if !removed {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(stateFile), 0o755); err != nil {
		return false, err
	}
	data, err = json.MarshalIndent(state, "", "  ")
	if err != nil {
		return false, err
	}
	return true, os.WriteFile(stateFile, data, 0o600)
}

func (f *failTracker) load() error {
	if f == nil {
		return nil
	}
	if f.stateFile != "" {
		data, err := os.ReadFile(f.stateFile)
		if err != nil {
			if !os.IsNotExist(err) {
				return err
			}
		} else {
			var state persistedRuntimeState
			if err := json.Unmarshal(data, &state); err != nil {
				return err
			}
			now := time.Now()
			f.mu.Lock()
			for _, item := range state.BlockedIPs {
				if item.IP == "" || !item.BlockedUntil.After(now) {
					continue
				}
				f.items[item.IP] = &failState{blockedUntil: item.BlockedUntil}
			}
			f.mu.Unlock()
		}
	}
	permanent, err := loadPermanentBlockedIPs(f.permanentFile)
	if err != nil {
		return err
	}
	f.mu.Lock()
	for ip := range permanent {
		f.permanent.Store(ip, struct{}{})
		delete(f.items, ip)
	}
	f.mu.Unlock()
	return nil
}

func (f *failTracker) saveLocked() error {
	if f == nil || f.stateFile == "" {
		return nil
	}
	now := time.Now()
	state := persistedRuntimeState{BlockedIPs: []persistedBlockedIP{}}
	for ip, item := range f.items {
		if item == nil || !item.blockedUntil.After(now) {
			continue
		}
		state.BlockedIPs = append(state.BlockedIPs, persistedBlockedIP{
			IP:           ip,
			BlockedUntil: item.blockedUntil,
		})
	}
	if err := os.MkdirAll(filepath.Dir(f.stateFile), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(f.stateFile, data, 0o600)
}

func (f *failTracker) snapshotBlocked() []BlockedIPState {
	if f == nil {
		return nil
	}
	now := time.Now()
	changed := false
	out := []BlockedIPState{}
	f.mu.Lock()
	defer f.mu.Unlock()
	for ip, item := range f.items {
		if item == nil || item.blockedUntil.IsZero() {
			continue
		}
		if !item.blockedUntil.After(now) {
			delete(f.items, ip)
			changed = true
			continue
		}
		out = append(out, BlockedIPState{
			IP:           ip,
			BlockedUntil: item.blockedUntil.Format(time.RFC3339),
			RemainingSec: int64(item.blockedUntil.Sub(now).Seconds()),
		})
	}
	if changed {
		_ = f.saveLocked()
	}
	return out
}

func (f *failTracker) unblock(key string) (bool, error) {
	if f == nil {
		return false, nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return false, nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	_, removed := f.items[key]
	delete(f.items, key)
	if err := f.saveLocked(); err != nil {
		return removed, err
	}
	return removed, nil
}

func blockedIPStates(items []persistedBlockedIP, now time.Time) []BlockedIPState {
	out := []BlockedIPState{}
	for _, item := range items {
		if item.IP == "" || item.BlockedUntil.IsZero() || !item.BlockedUntil.After(now) {
			continue
		}
		out = append(out, BlockedIPState{
			IP:           item.IP,
			BlockedUntil: item.BlockedUntil.Format(time.RFC3339),
			RemainingSec: int64(item.BlockedUntil.Sub(now).Seconds()),
		})
	}
	return out
}
