package labels

const (
	CastaiSpot            = "scheduling.cast.ai/spot"
	CastaiSpotFallback    = "scheduling.cast.ai/spot-fallback"
	CastaiFakeSpot        = "scheduling.cast.ai/fake-spot"
	KopsSpot              = "spot"
	KarpenterCapacityType = "karpenter.sh/capacity-type"
	WorkerSpot            = "node-role.kubernetes.io/spot-worker"

	ValueKarpenterCapacityTypeSpot     = "spot"
	ValueKarpenterCapacityTypeOnDemand = "on-demand"
)
