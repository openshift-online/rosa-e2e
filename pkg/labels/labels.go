package labels

import (
	"github.com/onsi/ginkgo/v2"
)

// Importance of test cases
var (
	Low      = ginkgo.Label("Importance:Low")
	Medium   = ginkgo.Label("Importance:Medium")
	High     = ginkgo.Label("Importance:High")
	Critical = ginkgo.Label("Importance:Critical")
)

// Positivity of test cases
var (
	Positive = ginkgo.Label("Positivity:Positive")
	Negative = ginkgo.Label("Positivity:Negative")
)

// Speed of test cases
var (
	Slow = ginkgo.Label("Speed:Slow")
)

// Platform labels (filter by product)
var (
	HCP     = ginkgo.Label("Platform:HCP")
	Classic = ginkgo.Label("Platform:Classic")
	OSDAWS  = ginkgo.Label("Platform:OSD-AWS")
	OSDGCP  = ginkgo.Label("Platform:OSD-GCP")
)

// Test area categories
var (
	ClusterLifecycle = ginkgo.Label("Area:ClusterLifecycle")
	DataPlane        = ginkgo.Label("Area:DataPlane")
	ManagedService   = ginkgo.Label("Area:ManagedService")
	CustomerFeatures = ginkgo.Label("Area:CustomerFeatures")
	Infrastructure   = ginkgo.Label("Area:Infrastructure")
	Upgrade          = ginkgo.Label("Area:Upgrade")
)
