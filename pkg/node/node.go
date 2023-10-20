package node

type NodeLifecycle string

const (
	NodeLifecycleOnDemand NodeLifecycle = "on-demand"
	NodeLifecycleSpot     NodeLifecycle = "spot"
)
