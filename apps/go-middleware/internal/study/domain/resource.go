package domain

import "time"

// ResourceType represents the type of a study resource.
type ResourceType string

const (
	ResourceTypeBook    ResourceType = "book"
	ResourceTypeNote    ResourceType = "note"
	ResourceTypeArticle ResourceType = "article"
	ResourceTypeVideo   ResourceType = "video"
)

// IsValid returns true if the ResourceType is one of the defined constants.
func (rt ResourceType) IsValid() bool {
	switch rt {
	case ResourceTypeBook, ResourceTypeNote, ResourceTypeArticle, ResourceTypeVideo:
		return true
	}
	return false
}

// Resource represents an external study resource (book, note, article, video)
// linked to a topic via TopicID and identified by a unique SourceURI.
type Resource struct {
	ID        string
	TopicID   string
	Title     string
	Type      ResourceType
	SourceURI string
	CreatedAt time.Time
}

// ResourceWithTopic is a resource plus the name of the topic it belongs to.
//
// The launcher shows the topic as the card's context line, so the read path
// resolves the name in the same query that reads the resource: a second lookup
// per row would be a per-item N+1 over a table the join already has open.
type ResourceWithTopic struct {
	Resource
	TopicName string
}
