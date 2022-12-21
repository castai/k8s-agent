package handlers

import (
	"castai-agent/internal/castai"
)

type Filter func(e castai.EventType, obj interface{}) bool
type Filters []Filter

func (fs Filters) apply(e castai.EventType, obj interface{}) bool {
	for _, f := range fs {
		f := f
		if !f(e, obj) {
			return false
		}
	}

	return true
}
