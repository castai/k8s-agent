//go:generate mockgen -source $GOFILE -destination ./mock/$GOFILE .
package monitor

type ProcessInfo interface {
	GetProcessStartTime() uint64
}
