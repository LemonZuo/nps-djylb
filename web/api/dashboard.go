package api

import (
	"net/http"

	"github.com/djylb/nps/server"
	"github.com/djylb/nps/server/tool"
)

// handleDashboard returns the overview counters and host metrics.
//
// The payload is server.GetDashboardData's map, passed through as-is. That map
// is assembled from a dozen sources with keys the previous UI already consumed
// (upTime, tcpCount, cpu, load, swap_mem, io_send, sys1..sys10, ...), and
// re-shaping it into a typed struct here would mean maintaining a second copy
// of that key list for no gain — the frontend reads the same names either way.
//
// force=true bypasses the 5-second cache. The UI polls this endpoint, so the
// default is the cached read; only an explicit refresh should pay for a full
// recompute.
func handleDashboard(w http.ResponseWriter, r *http.Request) {
	force := r.URL.Query().Get("force") == "true"
	Ok(w, r, server.GetDashboardData(force))
}

// handleDashboardHistory returns the sampled system metrics behind the charts.
//
// Sampling only runs when system_info_display is on (server/tool.StartSystemInfo
// checks it before starting the goroutine), so with it off this is an empty
// list rather than an error: the UI hides the chart, and a 404 here would show
// up as a failed request in the console on every dashboard load.
func handleDashboardHistory(w http.ResponseWriter, r *http.Request) {
	snapshot := tool.StatusSnapshot()
	if snapshot == nil {
		snapshot = []map[string]interface{}{}
	}
	OkList(w, r, snapshot, int64(len(snapshot)))
}
