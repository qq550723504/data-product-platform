// Adapted read-only from shuomi-ai-commerce@e29058c56df4294e1c612b3a705ab767bc333188; see docs/architecture/platform-management-readonly-v1.md.
package zitadel

import (
	"context"
	"time"

	domain "github.com/qq550723504/data-product-platform/apps/platform/internal/platformmanagement"
)

func (c *Client) List(ctx context.Context, organization string, request domain.PageRequest) (domain.Page, error) {
	request, err := request.Normalize()
	if err != nil {
		return domain.Page{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	filters := make([]any, 0, 2)
	if request.Filter.Role != "" {
		filters = append(filters, map[string]any{"roleKey": map[string]string{"key": request.Filter.Role}})
	}
	if request.Filter.State != "" {
		state := "STATE_ACTIVE"
		if request.Filter.State == "inactive" {
			state = "STATE_INACTIVE"
		}
		filters = append(filters, map[string]any{"state": map[string]string{"state": state}})
	}
	nativeFilter := request.Filter
	nativeFilter.Search = ""
	if request.Filter.Search == "" {
		page, err := c.list(ctx, organization, request, "", filters...)
		if err != nil {
			return domain.Page{}, err
		}
		if !completeFilteredPage(page, request, nativeFilter) {
			return domain.Page{}, domain.ErrInvalidResponse
		}
		return page, nil
	}

	// The provider's predicates are AND-only, and its preferred-login query
	// searches username rather than the returned preferredLoginName. Match both
	// returned fields only after traversing the entire bounded native result.
	items := make([]domain.Member, 0)
	total, previousID := -1, ""
	for offset := 0; offset < 10000; offset += 100 {
		if ctx.Err() != nil {
			return domain.Page{}, domain.ErrUnavailable
		}
		requestPage := domain.PageRequest{Limit: 100, Offset: offset}
		page, err := c.list(ctx, organization, requestPage, "", filters...)
		if err != nil {
			return domain.Page{}, err
		}
		if (total >= 0 && total != page.Total) || !completeFilteredPage(page, requestPage, nativeFilter) {
			return domain.Page{}, domain.ErrInvalidResponse
		}
		total = page.Total
		for _, member := range page.Items {
			if member.ID <= previousID {
				return domain.Page{}, domain.ErrInvalidResponse
			}
			previousID = member.ID
			if request.Filter.Matches(member) {
				items = append(items, member)
			}
		}
		if offset+len(page.Items) >= total {
			if ctx.Err() != nil {
				return domain.Page{}, domain.ErrUnavailable
			}
			start := min(request.Offset, len(items))
			end := min(start+request.Limit, len(items))
			return domain.Page{Items: items[start:end], Total: len(items)}, nil
		}
	}
	return domain.Page{}, domain.ErrInvalidResponse
}

func completeFilteredPage(page domain.Page, request domain.PageRequest, filter domain.ListFilter) bool {
	if len(page.Items) != min(request.Limit, max(0, page.Total-request.Offset)) {
		return false
	}
	for _, member := range page.Items {
		if !filter.Matches(member) {
			return false
		}
	}
	return true
}
