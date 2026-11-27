package view

import (
	"time"

	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/parse"
)

type ClaimItem struct {
	Holder    string `json:"holder"`
	ExpiresAt string `json:"expires_at"`
	Live      bool   `json:"live"`
}

func ClaimOf(st event.State, now time.Time) *ClaimItem {
	if st.Lease == nil {
		return nil
	}
	return &ClaimItem{Holder: st.Lease.Holder, ExpiresAt: parse.FormatTime(st.Lease.Expires), Live: st.Lease.LiveAt(now)}
}
