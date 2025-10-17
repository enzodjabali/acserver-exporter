package main

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
	PickupMode   bool     `json:"pickup"`
	Session      int      `json:"session"`
	SessionTypes []int    `json:"sessiontypes"`
	Country      []string `json:"country"`
	Pass         bool     `json:"pass"`
	Timestamp    int      `json:"timestamp"`
	TimeLeft     int      `json:"timeleft"`
	TimeOfDay    int      `json:"timeofday"`
	PoweredBy    string   `json:"poweredBy"`
}

// ACSP Protocol constants
const (
	ACSP_ERROR                = 0
	ACSP_CHAT                 = 1
	ACSP_CLIENT_LOADED        = 2
	ACSP_NEW_SESSION          = 3
	ACSP_NEW_CONNECTION       = 4
	ACSP_CONNECTION_CLOSED    = 5
	ACSP_CAR_UPDATE           = 6
	ACSP_CAR_INFO             = 7
	ACSP_END_SESSION          = 8
	ACSP_LAP_COMPLETED        = 9
	ACSP_VERSION              = 10
	ACSP_SESSION_INFO         = 11
	ACSP_CLIENT_EVENT         = 12
	ACSP_REALTIMEPOS_INTERVAL = 3
	ACSP_GET_CAR_INFO         = 4
	ACSP_GET_SESSION_INFO     = 7
)
