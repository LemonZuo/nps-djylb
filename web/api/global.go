package api

import (
	"net/http"

	"github.com/djylb/nps/lib/file"
)

// Global settings endpoints. The only persisted global today is the IP
// blacklist consulted by every proxy connection (server/proxy/base.go); the
// login ban list lives in memory and has its own routes in router.go.

// GlobalView is the settings document. The blacklist travels as a list, not a
// textarea blob — joining with newlines is presentation, and the stored form
// is already []string.
type GlobalView struct {
	BlackIPList []string `json:"blackIpList"`
}

// handleGetGlobal returns the global settings. Admin only.
func handleGetGlobal(w http.ResponseWriter, r *http.Request) {
	view := GlobalView{BlackIPList: []string{}}
	if g := file.GetDb().GetGlobal(); g != nil {
		g.RLock()
		view.BlackIPList = append(view.BlackIPList, g.BlackIpList...)
		g.RUnlock()
	}
	Ok(w, r, view)
}

// GlobalRequest replaces the global settings wholesale. There is one document,
// so this is a PUT, not a patch.
type GlobalRequest struct {
	BlackIPList []string `json:"blackIpList"`
}

// handleUpdateGlobal saves the global settings. Admin only.
func handleUpdateGlobal(w http.ResponseWriter, r *http.Request) {
	var req GlobalRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, r, err.Error())
		return
	}
	list := make([]string, 0, len(req.BlackIPList))
	seen := make(map[string]struct{}, len(req.BlackIPList))
	for _, v := range req.BlackIPList {
		if v == "" {
			continue
		}
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		list = append(list, v)
	}
	if err := file.GetDb().SaveGlobal(&file.Glob{BlackIpList: list}); err != nil {
		Internal(w, r, err)
		return
	}
	Ok(w, r, GlobalView{BlackIPList: list})
}
