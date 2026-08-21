package feishu

import "errors"

// The codes are on the error the API returned; the digits also appear in its
// text, and reading them there matched any message that happened to quote one
// while missing a send that carried the code in a field.
const (
	feishuCardTooLargeCode = 11310
	feishuCardInvalidCode  = 11325
)

func isCardLimitError(err error) bool {
	var apiErr *feishuAPIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.code == feishuCardTooLargeCode || apiErr.code == feishuCardInvalidCode
}
