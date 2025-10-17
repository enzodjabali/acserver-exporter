package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

type ACServerMonitor struct {
	conn               *net.UDPConn
	serverAddr         *net.UDPAddr
	httpHost           string
	httpPort           int
	cars               map[uint8]*CarInfo
	mu                 sync.RWMutex
	serverInfo         *ServerInfo
	serverName         string
	trackName          string
	sessionType        string
	
	// Metrics counters
	totalLaps          int64
	totalCollisions    int64
	totalConnections   int64
	totalDisconnections int64
	metricsLock        sync.RWMutex
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

func (m *ACServerMonitor) Connect() error {
	handshake := []byte{ACSP_REALTIMEPOS_INTERVAL}
	_, err := m.conn.WriteToUDP(handshake, m.serverAddr)
	if err != nil {
		return fmt.Errorf("handshake failed: %v", err)
	}

	sessionInfoReq := []byte{ACSP_GET_SESSION_INFO}
	_, err = m.conn.WriteToUDP(sessionInfoReq, m.serverAddr)
	if err != nil {
		return fmt.Errorf("session info request failed: %v", err)
	}

	fmt.Println("✓ Connected to Assetto Corsa server via UDP")
	return nil
}

func (m *ACServerMonitor) RequestCarInfo(carID uint8) error {
	req := []byte{ACSP_GET_CAR_INFO, carID}
	_, err := m.conn.WriteToUDP(req, m.serverAddr)
	return err
}

func (m *ACServerMonitor) GetCurrentStats() {
	if err := FetchHTTPInfo(m); err != nil {
		log.Printf("HTTP API error: %v", err)
	}
	
	for i := uint8(0); i < 50; i++ {
		m.RequestCarInfo(i)
		time.Sleep(10 * time.Millisecond)
	}
	
	time.Sleep(1 * time.Second)
	m.PrintStats()
}

func (m *ACServerMonitor) PrintStats() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	connectedCars := 0
	for _, car := range m.cars {
		if car.IsConnected {
			connectedCars++
		}
	}
	
	if m.serverInfo != nil {
		sessionNames := map[int]string{0: "Booking", 1: "Practice", 2: "Qualifying", 3: "Race"}
		sessionName := sessionNames[m.serverInfo.Session]
		
		fmt.Printf("Server: %s | Track: %s | Mode: %s | Players: %d/%d\n",
			m.serverInfo.Name, m.serverInfo.Track, sessionName,
			connectedCars, m.serverInfo.MaxClients)
	}
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
	}
}

func (m *ACServerMonitor) handleNewSession(data []byte) {
	if len(data) < 4 {
		return
	}
	
	reader := bytes.NewReader(data)
	var version, sessionIndex, currentSessionIndex, sessionCount uint8
	
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
	
	fmt.Printf("🏁 NEW SESSION: %s on %s\n", serverName, track)
}

func (m *ACServerMonitor) handleNewConnection(data []byte) {
	if len(data) < 1 {
		return
	}
	
	reader := bytes.NewReader(data)
	driverName := readString(reader)
	driverGUID := readString(reader)
	
	var carID, carModel, carSkin uint8
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
	m.mu.Unlock()
	
	m.metricsLock.Lock()
	m.totalConnections++
	m.metricsLock.Unlock()
	
	fmt.Printf("CONNECTED: %s (Car #%d)\n", driverName, carID)
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
	m.mu.Unlock()
	
	m.metricsLock.Lock()
	m.totalDisconnections++
	m.metricsLock.Unlock()
	
	fmt.Printf("DISCONNECTED: %s (Car #%d)\n", driverName, carID)
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
	
	m.metricsLock.Lock()
	m.totalLaps++
	m.metricsLock.Unlock()
	
	lapTimeSeconds := float64(lapTime) / 1000.0
	minutes := int(lapTimeSeconds / 60)
	seconds := lapTimeSeconds - float64(minutes*60)
	
	driverName := fmt.Sprintf("Car #%d", carID)
	m.mu.RLock()
	if m.cars[carID] != nil {
		driverName = m.cars[carID].DriverName
	}
	m.mu.RUnlock()
	
	fmt.Printf("LAP: %s - %02d:%06.3f\n", driverName, minutes, seconds)
}

func (m *ACServerMonitor) handleCarInfo(data []byte) {
	if len(data) < 1 {
		return
	}
	
	reader := bytes.NewReader(data)
	var carID, isConnected uint8
	
	binary.Read(reader, binary.LittleEndian, &carID)
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
	var version, sessionIndex, currentSessionIndex, sessionCount uint8
	
	binary.Read(reader, binary.LittleEndian, &version)
	binary.Read(reader, binary.LittleEndian, &sessionIndex)
	binary.Read(reader, binary.LittleEndian, &currentSessionIndex)
	binary.Read(reader, binary.LittleEndian, &sessionCount)
	
	serverName := readString(reader)
	var sessionType uint8
	var sessionTime, laps, waitTime uint16
	
	binary.Read(reader, binary.LittleEndian, &sessionType)
	binary.Read(reader, binary.LittleEndian, &sessionTime)
	binary.Read(reader, binary.LittleEndian, &laps)
	binary.Read(reader, binary.LittleEndian, &waitTime)
	
	_ = readString(reader)
	_ = readString(reader)
	_ = readString(reader)
	_ = readString(reader)
	
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
	var carID, eventType uint8
	
	binary.Read(reader, binary.LittleEndian, &carID)
	binary.Read(reader, binary.LittleEndian, &eventType)
	
	m.metricsLock.Lock()
	m.totalCollisions++
	m.metricsLock.Unlock()
	
	events := map[uint8]string{0: "Collision with ENV", 1: "Collision with CAR"}
	eventName := events[eventType]
	
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
	
	fmt.Printf("CHAT [%s]: %s\n", driverName, message)
}

func (m *ACServerMonitor) GetConnectedCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	count := 0
	for _, car := range m.cars {
		if car.IsConnected {
			count++
		}
	}
	return count
}

func (m *ACServerMonitor) Close() {
	if m.conn != nil {
		m.conn.Close()
	}
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
	
	return strings.TrimRight(string(strBytes), "\x00")
}
