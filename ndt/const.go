package ndt

// Message constants for the NDT protocol
const (
	SrvQueue         = byte(1)
	MsgLogin         = byte(2)
	TestPrepare      = byte(3)
	TestStart        = byte(4)
	TestMsg          = byte(5)
	TestFinalize     = byte(6)
	MsgError         = byte(7)
	MsgResults       = byte(8)
	MsgLogout        = byte(9)
	MsgWaiting       = byte(10)
	MsgExtendedLogin = byte(11)

	TestC2S    = 2
	TestS2C    = 4
	TestStatus = 16
)

// Message constants for use in their respective channels
const (
	ReadyC2S = float64(-1)
	ReadyS2C = float64(-1)
)
