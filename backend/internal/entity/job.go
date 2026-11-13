package entity

const (
	QueueInteractive = "interactive"
	QueueBulk        = "bulk"
)

type Job interface {
	Kind() string
}

// ActiveUnique is a job that is not queued again while an equal one waits or runs (TZ 10.6); a finished
// one does not stop it.
type ActiveUnique interface {
	Job
	UniqueWhileActive()
}
