package main

import (
	"fmt"
	"net/http"
	"strings"
)

var monitor *ACServerMonitor

func InitPrometheusMetrics(m *ACServerMonitor) {
	monitor = m
}

func PrometheusHandler(m *ACServerMonitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Refresh stats before serving metrics
		if err := FetchHTTPInfo(m); err == nil {
			// Successfully fetched HTTP info
		}
		
		var metrics strings.Builder
		
		// Help text and type declarations
		metrics.WriteString("# HELP ac_server_up Server availability (1 = up, 0 = down)\n")
		metrics.WriteString("# TYPE ac_server_up gauge\n")
		
		metrics.WriteString("# HELP ac_server_players Current number of connected players\n")
		metrics.WriteString("# TYPE ac_server_players gauge\n")
		
		metrics.WriteString("# HELP ac_server_max_players Maximum player capacity\n")
		metrics.WriteString("# TYPE ac_server_max_players gauge\n")
		
		metrics.WriteString("# HELP ac_server_session Current session type (0=Booking, 1=Practice, 2=Qualifying, 3=Race)\n")
		metrics.WriteString("# TYPE ac_server_session gauge\n")
		
		metrics.WriteString("# HELP ac_server_cars_available Number of available car models\n")
		metrics.WriteString("# TYPE ac_server_cars_available gauge\n")
		
		metrics.WriteString("# HELP ac_server_password_protected Whether server requires password\n")
		metrics.WriteString("# TYPE ac_server_password_protected gauge\n")
		
		metrics.WriteString("# HELP ac_server_pickup_mode Whether pickup mode is enabled\n")
		metrics.WriteString("# TYPE ac_server_pickup_mode gauge\n")
		
		metrics.WriteString("# HELP ac_server_time_left Time remaining in current session (seconds)\n")
		metrics.WriteString("# TYPE ac_server_time_left gauge\n")
		
		metrics.WriteString("# HELP ac_server_lap_completed_total Total laps completed\n")
		metrics.WriteString("# TYPE ac_server_lap_completed_total counter\n")
		
		metrics.WriteString("# HELP ac_server_collisions_total Total collision events\n")
		metrics.WriteString("# TYPE ac_server_collisions_total counter\n")
		
		metrics.WriteString("# HELP ac_server_connections_total Total player connections\n")
		metrics.WriteString("# TYPE ac_server_connections_total counter\n")
		
		metrics.WriteString("# HELP ac_server_disconnections_total Total player disconnections\n")
		metrics.WriteString("# TYPE ac_server_disconnections_total counter\n")
		
		m.mu.RLock()
		info := m.serverInfo
		m.mu.RUnlock()
		
		if info != nil {
			// Server is up
			labels := fmt.Sprintf(`server_name="%s",track="%s",powered_by="%s"`,
				escapeLabelValue(info.Name),
				escapeLabelValue(info.Track),
				escapeLabelValue(info.PoweredBy))
			
			metrics.WriteString(fmt.Sprintf("ac_server_up{%s} 1\n", labels))
			metrics.WriteString(fmt.Sprintf("ac_server_players{%s} %d\n", labels, info.Clients))
			metrics.WriteString(fmt.Sprintf("ac_server_max_players{%s} %d\n", labels, info.MaxClients))
			metrics.WriteString(fmt.Sprintf("ac_server_session{%s} %d\n", labels, info.Session))
			metrics.WriteString(fmt.Sprintf("ac_server_cars_available{%s} %d\n", labels, len(info.Cars)))
			
			passwordProtected := 0
			if info.Pass {
				passwordProtected = 1
			}
			metrics.WriteString(fmt.Sprintf("ac_server_password_protected{%s} %d\n", labels, passwordProtected))
			
			pickupMode := 0
			if info.PickupMode {
				pickupMode = 1
			}
			metrics.WriteString(fmt.Sprintf("ac_server_pickup_mode{%s} %d\n", labels, pickupMode))
			metrics.WriteString(fmt.Sprintf("ac_server_time_left{%s} %d\n", labels, info.TimeLeft))
		} else {
			// Server is down or unreachable
			metrics.WriteString("ac_server_up 0\n")
			metrics.WriteString("ac_server_players 0\n")
			metrics.WriteString("ac_server_max_players 0\n")
		}
		
		// Counters (these persist across scrapes)
		m.metricsLock.RLock()
		metrics.WriteString(fmt.Sprintf("ac_server_lap_completed_total %d\n", m.totalLaps))
		metrics.WriteString(fmt.Sprintf("ac_server_collisions_total %d\n", m.totalCollisions))
		metrics.WriteString(fmt.Sprintf("ac_server_connections_total %d\n", m.totalConnections))
		metrics.WriteString(fmt.Sprintf("ac_server_disconnections_total %d\n", m.totalDisconnections))
		m.metricsLock.RUnlock()
		
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.Write([]byte(metrics.String()))
	}
}

func escapeLabelValue(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}
