package loadbalancer

const (
	VIPClass     = "openyurt.io/vip"
	MockClass    = "openyurt.io/mock"
	UnknownClass = "openyurt.io/unknown"
)

const (
	name                  = "loadblancer"
	LBFinalizer           = "service.openyurt.io/loadbalancer"
	FailedAddFinalizer    = "FailedAddFinalizer"
	FailedRemoveFinalizer = "FailedRemoveFinalizer"
)
