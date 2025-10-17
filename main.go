package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	// Message types from server
	ACSP_ERROR                  = 0
	ACSP_CHAT                   = 1
	ACSP_CLIENT_LOADED          = 2
	ACSP_NEW_SESSION            = 3
	ACSP_NEW_CONNECTION         = 4
	ACSP_CONNECTION_CLOSED      = 5
	ACSP_CAR_UPDATE             = 6
	ACSP_CAR_INFO               = 7
	ACSP_END_SESSION            = 8
	ACSP_LAP_COMPLETED          = 9
	ACSP_VERSION                = 10
	ACSP_SESSION_INFO           = 11
	ACSP_CLIENT_EVENT           = 12

	// Operations for client to send
	ACSP_REALTIMEPOS_INTERVAL   = 3
	ACSP_GET_CAR_INFO           = 4
	ACSP_SEND_CHAT              = 5
	ACSP_BROADCAST_CHAT         = 6
	ACSP_GET_SESSION_INFO       = 7
	ACSP_SET_SESSION_INFO       = 8
	ACSP_KICK_USER              = 9
	ACSP_NEXT_SESSION           = 10
	ACSP_RESTART_SESSION        = 11
	ACSP_ADMIN_COMMAND          = 12
)

type CarInfo struct {
	CarID       uint8
	IsConnected bool
	CarModel    string
	CarSkin     string
	DriverName  string
	DriverGUID  string
}

type ServerInfo struct {
	Cars         []string `json:"cars"`
	Clients      int      `json:"clients"`
	Track        string   `json:"track"`
	Name         string   `json:"name"`
	MaxClients   int      `json:"maxclients"`
	Port         int      `json:"port"`
	PickupMode   bool     `json:"pickup_mode_enabled"`
	Session      int      `json:"session"`
	SessionTypes []int    `json:"sessiontypes"`
	TrackConfig  string   `json:"track_config"`
	TimeOfDay    int      `json:"time"`
	ElapsedMS    int      `json:"elapsed_ms"`
	Country      []string `json:"country"`
	Pass         bool     `json:"pass"`
	Timestamp    int      `json:"timestamp"`
}

type ACServerMonitor struct {
	conn       *net.UDPConn
	serverAddr *net.UDPAddr
	httpHost   string
	httpPort   int
	cars       map[uint8]*CarInfo
	mu         sync.RWMutex
	serverInfo *ServerInfo
	serverName string
	trackName  string
	sessionType string
}

func NewACServerMonitor(host string, udpPort int, httpPort int) (*ACServerMonitor, error) {
	serverAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", host, udpPort))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve UDP address: %v", err)
	}

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, fmt.Errorf("failed to create UDP connection: %v", err)
	}

	return &ACServerMonitor{
		conn:       conn,
		serverAddr: serverAddr,
		httpHost:   host,
		httpPort:   httpPort,
		cars:       make(map[uint8]*CarInfo),
	}, nil
}

func (m *ACServerMonitor) FetchHTTPInfo() error {
	url := fmt.Sprintf("http://%s:%d/INFO", m.httpHost, m.httpPort)
	
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("HTTP API request failed: %v", err)
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %v", err)
	}
	
	// Debug: print raw JSON
	fmt.Printf("📡 Raw server response:\n%s\n\n", string(body))
	
	var info ServerInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return fmt.Errorf("failed to parse JSON: %v", err)
	}
	
	m.mu.Lock()
	m.serverInfo = &info
	m.mu.Unlock()
	
	return nil
}

func (m *ACServerMonitor) Connect() error {
	// Send handshake - operation ID for connection
	handshake := []byte{ACSP_REALTIMEPOS_INTERVAL}
	_, err := m.conn.WriteToUDP(handshake, m.serverAddr)
	if err != nil {
		return fmt.Errorf("handshake failed: %v", err)
	}

	// Request session info
	sessionInfoReq := []byte{ACSP_GET_SESSION_INFO}
	_, err = m.conn.WriteToUDP(sessionInfoReq, m.serverAddr)
	if err != nil {
		return fmt.Errorf("session info request failed: %v", err)
	}

	fmt.Println("✓ Connected to Assetto Corsa server")
	return nil
}

func (m *ACServerMonitor) RequestCarInfo(carID uint8) error {
	req := []byte{ACSP_GET_CAR_INFO, carID}
	_, err := m.conn.WriteToUDP(req, m.serverAddr)
	return err
}

func (m *ACServerMonitor) GetCurrentStats() {
	fmt.Println("\n📊 Fetching server information...")
	
	// Try HTTP API first
	if err := m.FetchHTTPInfo(); err != nil {
		fmt.Printf("⚠️  HTTP API unavailable: %v\n", err)
	}
	
	// Request info for first 50 car slots
	for i := uint8(0); i < 50; i++ {
		m.RequestCarInfo(i)
		time.Sleep(10 * time.Millisecond)
	}
	
	// Wait for responses
	time.Sleep(2 * time.Second)
	
	m.PrintStats()
}

func (m *ACServerMonitor) PrintStats() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	connectedCars := 0
	var connectedDrivers []string
	
	for _, car := range m.cars {
		if car.IsConnected {
			connectedCars++
			connectedDrivers = append(connectedDrivers, fmt.Sprintf("  • %s (Car #%d) - %s", car.DriverName, car.CarID, car.CarModel))
		}
	}
	
	fmt.Println("\n╔════════════════════════════════════════════════════════════╗")
	
	// Display server info from HTTP API if available
	if m.serverInfo != nil {
		fmt.Printf("║ SERVER: %-50s ║\n", truncate(m.serverInfo.Name, 50))
		
		trackDisplay := m.serverInfo.Track
		if m.serverInfo.TrackConfig != "" {
			trackDisplay = fmt.Sprintf("%s (%s)", m.serverInfo.Track, m.serverInfo.TrackConfig)
		}
		fmt.Printf("║ TRACK:  %-50s ║\n", truncate(trackDisplay, 50))
		
		// Display session type - convert session type ID to name
		sessionNames := map[int]string{
			0: "Booking",
			1: "Practice",
			2: "Qualifying",
			3: "Race",
		}
		sessionName := "Unknown"
		if name, ok := sessionNames[m.serverInfo.Session]; ok {
			sessionName = name
		}
		fmt.Printf("║ MODE:   %-50s ║\n", truncate(sessionName, 50))
		
		// Show password protected
		passwordStatus := "No"
		if m.serverInfo.Pass {
			passwordStatus = "Yes"
		}
		fmt.Printf("║ 🔒 PASSWORD: %-45s ║\n", passwordStatus)
		
		fmt.Println("╠════════════════════════════════════════════════════════════╣")
		fmt.Printf("║ 👥 PLAYERS: %d / %-42d ║\n", m.serverInfo.Clients, m.serverInfo.MaxClients)
		
		// Display available cars
		if len(m.serverInfo.Cars) > 0 {
			// Show first few cars
			carsToShow := m.serverInfo.Cars
			if len(carsToShow) > 3 {
				carsToShow = carsToShow[:3]
				carsDisplay := strings.Join(carsToShow, ", ") + fmt.Sprintf(" +%d more", len(m.serverInfo.Cars)-3)
				fmt.Printf("║ 🚗 CARS:    %-48s ║\n", truncate(carsDisplay, 48))
			} else {
				carsDisplay := strings.Join(carsToShow, ", ")
				fmt.Printf("║ 🚗 CARS:    %-48s ║\n", truncate(carsDisplay, 48))
			}
			fmt.Printf("║            Total: %-42d ║\n", len(m.serverInfo.Cars))
		}
		
		// Display pickup mode
		pickupMode := "No"
		if m.serverInfo.PickupMode {
			pickupMode = "Yes"
		}
		fmt.Printf("║ 🔄 PICKUP:  %-48s ║\n", pickupMode)
		
		// Display port
		fmt.Printf("║ 🔌 PORT:    %-48d ║\n", m.serverInfo.Port)
		
	} else {
		// Fallback to UDP data
		name := m.serverName
		if name == "" {
			name = "Unknown"
		}
		fmt.Printf("║ SERVER: %-50s ║\n", truncate(name, 50))
		
		if m.trackName != "" {
			fmt.Printf("║ TRACK:  %-50s ║\n", truncate(m.trackName, 50))
		}
		if m.sessionType != "" {
			fmt.Printf("║ MODE:   %-50s ║\n", truncate(m.sessionType, 50))
		}
		fmt.Println("╠════════════════════════════════════════════════════════════╣")
		fmt.Printf("║ 👥 CONNECTED PLAYERS: %-37d ║\n", connectedCars)
	}
	
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	
	if connectedCars > 0 {
		fmt.Println("\n🚗 Connected Drivers:")
		for _, driver := range connectedDrivers {
			fmt.Println(driver)
		}
	} else {
		fmt.Println("\n⚠️  No players currently connected")
	}
	
	fmt.Println("\n" + strings.Repeat("─", 60))
	fmt.Println("Monitoring live events...")
	fmt.Println(strings.Repeat("─", 60) + "\n")
}

func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}

func (m *ACServerMonitor) Listen() {
	buffer := make([]byte, 2048)

	for {
		n, _, err := m.conn.ReadFromUDP(buffer)
		if err != nil {
			log.Printf("Error reading UDP: %v", err)
			continue
		}

		if n > 0 {
			m.handleMessage(buffer[:n])
		}
	}
}

func (m *ACServerMonitor) handleMessage(data []byte) {
	if len(data) == 0 {
		return
	}

	msgType := data[0]
	
	switch msgType {
	case ACSP_NEW_SESSION:
		m.handleNewSession(data[1:])
	case ACSP_NEW_CONNECTION:
		m.handleNewConnection(data[1:])
	case ACSP_CONNECTION_CLOSED:
		m.handleConnectionClosed(data[1:])
	case ACSP_LAP_COMPLETED:
		m.handleLapCompleted(data[1:])
	case ACSP_CAR_INFO:
		m.handleCarInfo(data[1:])
	case ACSP_SESSION_INFO:
		m.handleSessionInfo(data[1:])
	case ACSP_CLIENT_EVENT:
		m.handleClientEvent(data[1:])
	case ACSP_CHAT:
		m.handleChat(data[1:])
	case ACSP_VERSION:
		m.handleVersion(data[1:])
	case ACSP_ERROR:
		fmt.Printf("⚠️  Server Error\n")
	}
}

func (m *ACServerMonitor) handleNewSession(data []byte) {
	if len(data) < 4 {
		return
	}
	
	reader := bytes.NewReader(data)
	var version uint8
	var sessionIndex uint8
	var currentSessionIndex uint8
	var sessionCount uint8
	
	binary.Read(reader, binary.LittleEndian, &version)
	binary.Read(reader, binary.LittleEndian, &sessionIndex)
	binary.Read(reader, binary.LittleEndian, &currentSessionIndex)
	binary.Read(reader, binary.LittleEndian, &sessionCount)
	
	serverName := readString(reader)
	track := readString(reader)
	trackConfig := readString(reader)
	
	m.mu.Lock()
	m.serverName = serverName
	m.trackName = fmt.Sprintf("%s (%s)", track, trackConfig)
	m.mu.Unlock()
	
	fmt.Printf("\n🏁 NEW SESSION\n")
	fmt.Printf("   Server: %s\n", serverName)
	fmt.Printf("   Track: %s (%s)\n", track, trackConfig)
	fmt.Printf("   Session: %d/%d\n\n", currentSessionIndex+1, sessionCount)
	
	// Request fresh stats after session change
	go func() {
		time.Sleep(1 * time.Second)
		m.GetCurrentStats()
	}()
}

func (m *ACServerMonitor) handleNewConnection(data []byte) {
	if len(data) < 1 {
		return
	}
	
	reader := bytes.NewReader(data)
	driverName := readString(reader)
	driverGUID := readString(reader)
	
	var carID uint8
	var carModel uint8
	var carSkin uint8
	
	binary.Read(reader, binary.LittleEndian, &carID)
	binary.Read(reader, binary.LittleEndian, &carModel)
	binary.Read(reader, binary.LittleEndian, &carSkin)
	
	m.mu.Lock()
	if m.cars[carID] == nil {
		m.cars[carID] = &CarInfo{}
	}
	m.cars[carID].CarID = carID
	m.cars[carID].IsConnected = true
	m.cars[carID].DriverName = driverName
	m.cars[carID].DriverGUID = driverGUID
	connectedCount := m.getConnectedCount()
	maxClients := 0
	if m.serverInfo != nil {
		maxClients = m.serverInfo.MaxClients
	}
	m.mu.Unlock()
	
	if maxClients > 0 {
		fmt.Printf("👤 DRIVER CONNECTED: %s (Car #%d) | Players: %d/%d\n", driverName, carID, connectedCount, maxClients)
	} else {
		fmt.Printf("👤 DRIVER CONNECTED: %s (Car #%d) | Total players: %d\n", driverName, carID, connectedCount)
	}
}

func (m *ACServerMonitor) handleConnectionClosed(data []byte) {
	if len(data) < 1 {
		return
	}
	
	reader := bytes.NewReader(data)
	driverName := readString(reader)
	
	var carID uint8
	binary.Read(reader, binary.LittleEndian, &carID)
	
	m.mu.Lock()
	if m.cars[carID] != nil {
		m.cars[carID].IsConnected = false
	}
	connectedCount := m.getConnectedCount()
	maxClients := 0
	if m.serverInfo != nil {
		maxClients = m.serverInfo.MaxClients
	}
	m.mu.Unlock()
	
	if maxClients > 0 {
		fmt.Printf("👋 DRIVER DISCONNECTED: %s (Car #%d) | Players: %d/%d\n", driverName, carID, connectedCount, maxClients)
	} else {
		fmt.Printf("👋 DRIVER DISCONNECTED: %s (Car #%d) | Total players: %d\n", driverName, carID, connectedCount)
	}
}

func (m *ACServerMonitor) getConnectedCount() int {
	count := 0
	for _, car := range m.cars {
		if car.IsConnected {
			count++
		}
	}
	return count
}

func (m *ACServerMonitor) handleLapCompleted(data []byte) {
	if len(data) < 9 {
		return
	}
	
	reader := bytes.NewReader(data)
	
	var carID uint8
	var lapTime uint32
	var cuts uint8
	
	binary.Read(reader, binary.LittleEndian, &carID)
	binary.Read(reader, binary.LittleEndian, &lapTime)
	binary.Read(reader, binary.LittleEndian, &cuts)
	
	lapTimeSeconds := float64(lapTime) / 1000.0
	minutes := int(lapTimeSeconds / 60)
	seconds := lapTimeSeconds - float64(minutes*60)
	
	cutsText := ""
	if cuts > 0 {
		cutsText = fmt.Sprintf(" [%d cuts]", cuts)
	}
	
	driverName := fmt.Sprintf("Car #%d", carID)
	m.mu.RLock()
	if m.cars[carID] != nil {
		driverName = m.cars[carID].DriverName
	}
	m.mu.RUnlock()
	
	fmt.Printf("⏱️  LAP COMPLETED: %s - %02d:%06.3f%s\n", driverName, minutes, seconds, cutsText)
}

func (m *ACServerMonitor) handleCarInfo(data []byte) {
	if len(data) < 1 {
		return
	}
	
	reader := bytes.NewReader(data)
	var carID uint8
	binary.Read(reader, binary.LittleEndian, &carID)
	
	var isConnected uint8
	binary.Read(reader, binary.LittleEndian, &isConnected)
	
	carModel := readString(reader)
	carSkin := readString(reader)
	driverName := readString(reader)
	driverGUID := readString(reader)
	
	m.mu.Lock()
	m.cars[carID] = &CarInfo{
		CarID:       carID,
		IsConnected: isConnected == 1,
		CarModel:    carModel,
		CarSkin:     carSkin,
		DriverName:  driverName,
		DriverGUID:  driverGUID,
	}
	m.mu.Unlock()
}

func (m *ACServerMonitor) handleSessionInfo(data []byte) {
	if len(data) < 13 {
		return
	}
	
	reader := bytes.NewReader(data)
	var version uint8
	binary.Read(reader, binary.LittleEndian, &version)
	
	var sessionIndex uint8
	var currentSessionIndex uint8
	var sessionCount uint8
	
	binary.Read(reader, binary.LittleEndian, &sessionIndex)
	binary.Read(reader, binary.LittleEndian, &currentSessionIndex)
	binary.Read(reader, binary.LittleEndian, &sessionCount)
	
	serverName := readString(reader)
	
	var sessionType uint8
	binary.Read(reader, binary.LittleEndian, &sessionType)
	
	var sessionTime uint16
	var laps uint16
	var waitTime uint16
	
	binary.Read(reader, binary.LittleEndian, &sessionTime)
	binary.Read(reader, binary.LittleEndian, &laps)
	binary.Read(reader, binary.LittleEndian, &waitTime)
	
	_ = readString(reader) // ambientTemp
	_ = readString(reader) // roadTemp
	_ = readString(reader) // weatherGraphics
	_ = readString(reader) // elapsedMS
	
	sessionTypes := []string{"Practice", "Qualifying", "Race"}
	sessionTypeName := "Unknown"
	if int(sessionType) < len(sessionTypes) {
		sessionTypeName = sessionTypes[sessionType]
	}
	
	m.mu.Lock()
	m.serverName = serverName
	m.sessionType = sessionTypeName
	m.mu.Unlock()
}

func (m *ACServerMonitor) handleClientEvent(data []byte) {
	if len(data) < 2 {
		return
	}
	
	reader := bytes.NewReader(data)
	var carID uint8
	var eventType uint8
	
	binary.Read(reader, binary.LittleEndian, &carID)
	binary.Read(reader, binary.LittleEndian, &eventType)
	
	events := map[uint8]string{
		0: "Collision with ENV",
		1: "Collision with CAR",
	}
	
	eventName := events[eventType]
	if eventName == "" {
		eventName = fmt.Sprintf("Unknown (%d)", eventType)
	}
	
	driverName := fmt.Sprintf("Car #%d", carID)
	m.mu.RLock()
	if m.cars[carID] != nil {
		driverName = m.cars[carID].DriverName
	}
	m.mu.RUnlock()
	
	fmt.Printf("⚡ EVENT: %s - %s\n", driverName, eventName)
}

func (m *ACServerMonitor) handleChat(data []byte) {
	if len(data) < 1 {
		return
	}
	
	reader := bytes.NewReader(data)
	var carID uint8
	binary.Read(reader, binary.LittleEndian, &carID)
	
	message := readString(reader)
	
	driverName := fmt.Sprintf("Car #%d", carID)
	m.mu.RLock()
	if m.cars[carID] != nil {
		driverName = m.cars[carID].DriverName
	}
	m.mu.RUnlock()
	
	fmt.Printf("💬 CHAT [%s]: %s\n", driverName, message)
}

func (m *ACServerMonitor) handleVersion(data []byte) {
	if len(data) < 1 {
		return
	}
	
	reader := bytes.NewReader(data)
	var version uint8
	binary.Read(reader, binary.LittleEndian, &version)
	
	fmt.Printf("ℹ️  Protocol Version: %d\n", version)
}

func readString(reader *bytes.Reader) string {
	var length uint8
	err := binary.Read(reader, binary.LittleEndian, &length)
	if err != nil || length == 0 {
		return ""
	}
	
	strBytes := make([]byte, length)
	_, err = reader.Read(strBytes)
	if err != nil {
		return ""
	}
	
	return string(strBytes)
}

func (m *ACServerMonitor) Close() {
	if m.conn != nil {
		m.conn.Close()
	}
}

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
	
	fmt.Printf("Connecting to %s (UDP:%d, HTTP:%d)...\n", host, udpPort, httpPort)
	
	monitor, err := NewACServerMonitor(host, udpPort, httpPort)
	if err != nil {
		log.Fatalf("Failed to create monitor: %v", err)
	}
	defer monitor.Close()
	
	if err := monitor.Connect(); err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	
	// Start listening in background
	go monitor.Listen()
	
	// Wait for initial session info
	time.Sleep(1 * time.Second)
	
	// Get current stats
	monitor.GetCurrentStats()
	
	// Refresh stats every 30 seconds
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			monitor.GetCurrentStats()
		}
	}()
	
	// Keep running and monitoring events
	select {}
}
