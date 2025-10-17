package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
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

type ACServerMonitor struct {
	conn       *net.UDPConn
	serverAddr *net.UDPAddr
	cars       map[uint8]string
}

func NewACServerMonitor(host string, port int) (*ACServerMonitor, error) {
	serverAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", host, port))
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
		cars:       make(map[uint8]string),
	}, nil
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

func (m *ACServerMonitor) Listen() {
	buffer := make([]byte, 2048)
	
	fmt.Println("\n=== Assetto Corsa Server Monitor ===")
	fmt.Println("Listening for events...\n")

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
	default:
		// Uncomment for debugging unknown message types
		// fmt.Printf("Unknown message type: %d\n", msgType)
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
	
	fmt.Printf("🏁 NEW SESSION\n")
	fmt.Printf("   Server: %s\n", serverName)
	fmt.Printf("   Track: %s (%s)\n", track, trackConfig)
	fmt.Printf("   Session: %d/%d\n\n", currentSessionIndex+1, sessionCount)
}

func (m *ACServerMonitor) handleNewConnection(data []byte) {
	if len(data) < 1 {
		return
	}
	
	reader := bytes.NewReader(data)
	driverName := readString(reader)
	_ = readString(reader) // driverGUID - not displayed
	
	var carID uint8
	var carModel uint8
	var carSkin uint8
	
	binary.Read(reader, binary.LittleEndian, &carID)
	binary.Read(reader, binary.LittleEndian, &carModel)
	binary.Read(reader, binary.LittleEndian, &carSkin)
	
	fmt.Printf("👤 DRIVER CONNECTED: %s (Car #%d)\n", driverName, carID)
}

func (m *ACServerMonitor) handleConnectionClosed(data []byte) {
	if len(data) < 1 {
		return
	}
	
	reader := bytes.NewReader(data)
	driverName := readString(reader)
	
	var carID uint8
	binary.Read(reader, binary.LittleEndian, &carID)
	
	fmt.Printf("👋 DRIVER DISCONNECTED: %s (Car #%d)\n", driverName, carID)
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
	
	// Read number of cars (to skip grip data)
	var carsCount uint8
	binary.Read(reader, binary.LittleEndian, &carsCount)
	
	lapTimeSeconds := float64(lapTime) / 1000.0
	minutes := int(lapTimeSeconds / 60)
	seconds := lapTimeSeconds - float64(minutes*60)
	
	cutsText := ""
	if cuts > 0 {
		cutsText = fmt.Sprintf(" [%d cuts]", cuts)
	}
	
	fmt.Printf("⏱️  LAP COMPLETED: Car #%d - %02d:%06.3f%s\n", carID, minutes, seconds, cutsText)
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
	_ = readString(reader) // carSkin - not displayed
	driverName := readString(reader)
	
	m.cars[carID] = driverName
	
	status := "Connected"
	if isConnected == 0 {
		status = "Disconnected"
	}
	
	fmt.Printf("🚗 CAR INFO: #%d - %s driving %s [%s]\n", carID, driverName, carModel, status)
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
	
	sessionTypes := []string{"Practice", "Qualifying", "Race"}
	sessionTypeName := "Unknown"
	if int(sessionType) < len(sessionTypes) {
		sessionTypeName = sessionTypes[sessionType]
	}
	
	fmt.Printf("ℹ️  SESSION INFO\n")
	fmt.Printf("   Server: %s\n", serverName)
	fmt.Printf("   Type: %s\n", sessionTypeName)
	fmt.Printf("   Time: %d minutes | Laps: %d\n\n", sessionTime, laps)
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
	
	fmt.Printf("⚡ EVENT: Car #%d - %s\n", carID, eventName)
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
	if name, ok := m.cars[carID]; ok {
		driverName = name
	}
	
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
	
	port := 11000
	if portEnv := os.Getenv("AC_SERVER_UDP_PORT"); portEnv != "" {
		fmt.Sscanf(portEnv, "%d", &port)
	}
	
	fmt.Printf("Connecting to %s:%d...\n", host, port)
	
	monitor, err := NewACServerMonitor(host, port)
	if err != nil {
		log.Fatalf("Failed to create monitor: %v", err)
	}
	defer monitor.Close()
	
	if err := monitor.Connect(); err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	
	// Give server time to respond
	time.Sleep(1 * time.Second)
	
	monitor.Listen()
}
