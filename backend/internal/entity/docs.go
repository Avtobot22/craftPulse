package entity

// DocumentationRoot binds a service to a path with an explicitly assigned owner.
type DocumentationRoot struct {
	ID            ID
	ServiceID     ID
	AreaID        ID
	PathPrefix    string
	ProfileID     string
	OwnerPersonID *ID
	Audience      string
	Knowledge
	Confirmation
}

type DocumentationRootSet struct {
	ServiceID   ID
	Version     int64
	KnownSeq    int64
	ContentHash []byte
	Confirmation
	Roots []DocumentationRoot
}
