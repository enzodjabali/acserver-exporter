package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	host := os.Getenv("AC_SERVER_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	
	udpPort := 9600
	if portEnv := os.Getenv("AC_SERVER_UDP_PORT"); portEnv != "" {
		fmt.Sscanf(portEnv, "%d", &udpPort)
	}
	
	httpPort := 8081
	if portEnv := os.Getenv("AC_SERVER_HTTP_PORT"); portEnv != "" {
		fmt.Sscanf(portEnv, "%d", &httpPort)
	}
	
	metricsPort := os.Getenv("METRICS_PORT")
	if metricsPort == "" {
		metricsPort = "9090"
	}
	
	fmt.Printf("🚀 Starting Assetto Corsa Prometheus Exporter\n")
	fmt.Printf("   Target Server: %s (UDP:%d, HTTP:%d)\n", host, udpPort, httpPort)
	fmt.Printf("   Metrics Port: %s\n\n", metricsPort)
	
	// Create monitor
	monitor, err := NewACServerMonitor(host, udpPort, httpPort)
	if err != nil {
		log.Fatalf("Failed to create monitor: %v", err)
	}
	defer monitor.Close()
	
	// Connect to UDP
	if err := monitor.Connect(); err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	
	// Start UDP listener in background
	go monitor.Listen()
	
	// Initial stats fetch
	time.Sleep(1 * time.Second)
	monitor.GetCurrentStats()
	
	// Periodic stats refresh
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			monitor.GetCurrentStats()
		}
	}()
	
	// Setup Prometheus metrics
	InitPrometheusMetrics(monitor)
	
	// Setup HTTP server for metrics
	http.Handle("/metrics", PrometheusHandler(monitor))
	http.HandleFunc("/health", HealthHandler)
	http.HandleFunc("/", IndexHandler)
	
	fmt.Printf("✓ Metrics endpoint available at http://localhost:%s/metrics\n", metricsPort)
	fmt.Printf("✓ Health check available at http://localhost:%s/health\n\n", metricsPort)
	
	if err := http.ListenAndServe(":"+metricsPort, nil); err != nil {
		log.Fatalf("Failed to start HTTP server: %v", err)
	}
}

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func IndexHandler(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>Assetto Corsa Exporter</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; background: #f5f5f5; }
        .container { max-width: 800px; margin: 0 auto; background: white; padding: 30px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        h1 { color: #333; }
        .links { margin-top: 20px; }
        .links a { display: inline-block; margin-right: 15px; padding: 10px 20px; background: #007bff; color: white; text-decoration: none; border-radius: 4px; }
        .links a:hover { background: #0056b3; }
        pre { background: #f8f9fa; padding: 15px; border-radius: 4px; overflow-x: auto; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🏎️ Assetto Corsa Prometheus Exporter</h1>
        <p>This exporter collects metrics from an Assetto Corsa server and exposes them in Prometheus format.</p>
        
        <div class="links">
            <a href="/metrics">📊 Metrics</a>
            <a href="/health">💚 Health</a>
        </div>
        
        <h2>Available Metrics</h2>
        <ul>
            <li><code>ac_server_up</code> - Server availability (1 = up, 0 = down)</li>
            <li><code>ac_server_players</code> - Current number of connected players</li>
            <li><code>ac_server_max_players</code> - Maximum player capacity</li>
            <li><code>ac_server_session</code> - Current session type (0=Booking, 1=Practice, 2=Qualifying, 3=Race)</li>
            <li><code>ac_server_cars_available</code> - Number of available car models</li>
            <li><code>ac_server_password_protected</code> - Whether server requires password (1 = yes, 0 = no)</li>
            <li><code>ac_server_pickup_mode</code> - Whether pickup mode is enabled (1 = yes, 0 = no)</li>
            <li><code>ac_server_time_left</code> - Time remaining in current session (seconds)</li>
            <li><code>ac_server_lap_completed_total</code> - Total laps completed</li>
            <li><code>ac_server_collisions_total</code> - Total collision events</li>
            <li><code>ac_server_connections_total</code> - Total player connections</li>
            <li><code>ac_server_disconnections_total</code> - Total player disconnections</li>
        </ul>
    </div>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}
