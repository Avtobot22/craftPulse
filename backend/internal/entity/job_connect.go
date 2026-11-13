package entity

const KindConnectProbe = "connect.probe"

// ConnectProbeJob runs the probe of a connection in the interactive queue (UC-04).
type ConnectProbeJob struct {
	OrganizationID ID `json:"organization_id"`
	ConnectionID   ID `json:"connection_id"`
	ProbeID        ID `json:"probe_id"`
	CorrelationID  ID `json:"correlation_id"`
}

func (ConnectProbeJob) Kind() string { return KindConnectProbe }
