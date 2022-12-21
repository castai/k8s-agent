package transformers

import (
	"castai-agent/internal/castai"
)

type Transformer func(e castai.EventType, obj interface{}) (castai.EventType, interface{})

type Transformers []Transformer

func (ts Transformers) Apply(e castai.EventType, obj interface{}) (castai.EventType, interface{}) {
	for _, t := range ts {
		t := t
		e, obj = t(e, obj)
	}

	return e, obj
}
