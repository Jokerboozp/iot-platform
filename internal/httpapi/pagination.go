package httpapi

import "net/http"

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

type listPagination struct {
	Page     int
	PageSize int
	Offset   int
}

// parseListPagination accepts the new page/pageSize contract and the legacy
// limit/offset pair used by older clients. The server always caps one request
// so a caller cannot accidentally turn a list endpoint back into a full-table
// query.
func parseListPagination(r *http.Request) listPagination {
	query := r.URL.Query()
	pageSize := intval(query.Get("pageSize"), 0)
	if pageSize <= 0 {
		pageSize = intval(query.Get("limit"), defaultPageSize)
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	pageNumber := intval(query.Get("page"), 0)
	if pageNumber > 0 {
		return listPagination{Page: pageNumber, PageSize: pageSize, Offset: (pageNumber - 1) * pageSize}
	}

	offset := intval(query.Get("offset"), 0)
	if offset < 0 {
		offset = 0
	}
	return listPagination{Page: offset/pageSize + 1, PageSize: pageSize, Offset: offset}
}

func writeList(w http.ResponseWriter, status int, items any, total int, pagination listPagination, extra map[string]any) {
	response := map[string]any{
		"items":    items,
		"count":    total,
		"total":    total,
		"page":     pagination.Page,
		"pageSize": pagination.PageSize,
		"limit":    pagination.PageSize,
		"offset":   pagination.Offset,
		"pagination": map[string]int{
			"page":     pagination.Page,
			"pageSize": pagination.PageSize,
			"total":    total,
		},
	}
	for key, value := range extra {
		response[key] = value
	}
	write(w, status, response)
}

func pageItems[T any](items []T, pagination listPagination) ([]T, int) {
	total := len(items)
	if pagination.Offset >= total {
		return []T{}, total
	}
	end := pagination.Offset + pagination.PageSize
	if end > total {
		end = total
	}
	return items[pagination.Offset:end], total
}
