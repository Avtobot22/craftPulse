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

const KindRetentionTick = "retention.tick"

// RetentionTickJob deletes once a day what the retention table keeps for a while only (TZ 12.9).
type RetentionTickJob struct{}

func (RetentionTickJob) Kind() string { return KindRetentionTick }
