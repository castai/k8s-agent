package castai

import "fmt"

type DeltaRequestError struct {
	requestID  string
	statusCode int
	body       string
}

func (e DeltaRequestError) Error() string {
	return fmt.Sprintf("delta request error: request_id=%q status_code=%d body=%s", e.requestID, e.statusCode, e.body)
}
