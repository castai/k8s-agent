package filters

import (
	"castai-agent/internal/castai"
)

type Filter func(e castai.EventType, obj interface{}) bool
type Filters [][]Filter

func (fs Filters) Apply(e castai.EventType, obj interface{}) bool {
	for _, f := range fs {
		if fs.applyFilterSlice(e, obj, f) {
			return true
		}
	}
	return false
}

func (fs Filters) applyFilterSlice(e castai.EventType, obj interface{}, filters []Filter) bool {
	for _, f := range filters {
		if !f(e, obj) {
			return false
		}
	}
	return true
}
