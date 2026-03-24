package models

// PaginationQuery represents pagination parameters for list queries
type PaginationQuery struct {
	Page     int `form:"page" binding:"min=1"`
	PageSize int `form:"pageSize" binding:"min=1,max=100"`
}

// PaginationMeta represents pagination metadata in responses
type PaginationMeta struct {
	Total      int64 `json:"total"`
	Page       int   `json:"page"`
	PageSize   int   `json:"pageSize"`
	TotalPages int   `json:"totalPages"`
}

// UserListResult wraps user list with pagination metadata
type UserListResult struct {
	Users      []User         `json:"users"`
	Pagination PaginationMeta `json:"pagination"`
}

// DefaultPage is the default page number
const DefaultPage = 1

// DefaultPageSize is the default page size
const DefaultPageSize = 20

// MaxPageSize is the maximum allowed page size
const MaxPageSize = 100

// NormalizePagination normalizes pagination parameters to valid ranges
func NormalizePagination(page, pageSize int) (int, int) {
	if page < 1 {
		page = DefaultPage
	}
	if pageSize < 1 {
		pageSize = DefaultPageSize
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}
	return page, pageSize
}

// CalculateTotalPages calculates total pages from total count and page size
func CalculateTotalPages(total int64, pageSize int) int {
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}
	pages := int(total) / pageSize
	if int(total)%pageSize != 0 {
		pages++
	}
	if pages == 0 {
		pages = 1
	}
	return pages
}
