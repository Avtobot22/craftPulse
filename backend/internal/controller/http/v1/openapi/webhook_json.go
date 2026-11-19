package openapi

import "encoding/json"

// MarshalJSON keeps the nullable webhook address present even for an unsupported adapter.
func (c Connection) MarshalJSON() ([]byte, error) {
	type connection Connection
	return json.Marshal(struct {
		connection
		WebhookURL *string `json:"webhook_url"`
	}{connection: connection(c), WebhookURL: c.WebhookUrl})
}

// MarshalJSON keeps empty webhook diagnostics distinct from an absent response field.
func (s GetSyncStatus200JSONResponse) MarshalJSON() ([]byte, error) {
	type status SyncStatus
	return json.Marshal(struct {
		status
		Webhooks any `json:"webhooks"`
	}{status: status(s), Webhooks: s.Webhooks})
}
