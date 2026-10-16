package httpx

// Default and maximum page sizes for list endpoints.
const (
	DefaultPageLimit = 20
	MaxPageLimit     = 100
)

// PageQuery is embedded in list request DTOs.
type PageQuery struct {
	Limit  int `query:"limit" validate:"omitempty,min=1,max=100"`
	Offset int `query:"offset" validate:"omitempty,min=0"`
}

// Normalize applies defaults; call it after BindQuery.
func (p PageQuery) Normalize() PageQuery {
	if p.Limit <= 0 {
		p.Limit = DefaultPageLimit
	}
	if p.Limit > MaxPageLimit {
		p.Limit = MaxPageLimit
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	return p
}

// Page is the pagination metadata returned by list endpoints.
type Page struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}

// List is the envelope of every list response: {"data": [...], "page": {...}}.
type List[T any] struct {
	Data []T  `json:"data"`
	Page Page `json:"page"`
}
