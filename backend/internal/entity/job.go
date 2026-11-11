package entity

const (
	QueueInteractive = "interactive"
	QueueBulk        = "bulk"
)

type Job interface {
	Kind() string
}
