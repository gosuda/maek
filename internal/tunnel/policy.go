package tunnel

import (
	"fmt"

	"github.com/gosuda/maek/internal/protocol"
)

// FlowPolicy contains the current stream scheduling and flow-control defaults.
// It intentionally lives behind the tunnel implementation so these values can
// become adaptive per session or per stream without changing the wire format.
type FlowPolicy struct {
	MaxFramePayload   int
	InitialWindow     uint32
	WindowUpdateBatch uint32
	AcceptBacklog     int
}

func DefaultFlowPolicy() FlowPolicy {
	return FlowPolicy{
		MaxFramePayload:   16 << 10,
		InitialWindow:     1 << 20,
		WindowUpdateBatch: 256 << 10,
		AcceptBacklog:     256,
	}
}

func (p FlowPolicy) validate() error {
	if p.MaxFramePayload <= 0 || p.MaxFramePayload > protocol.MaxDataFramePayload {
		return fmt.Errorf("max frame payload must be between 1 and %d", protocol.MaxDataFramePayload)
	}
	if p.InitialWindow == 0 || p.InitialWindow > protocol.MaxStreamWindowCredit {
		return fmt.Errorf("initial window must be between 1 and %d", protocol.MaxStreamWindowCredit)
	}
	if p.WindowUpdateBatch == 0 || p.WindowUpdateBatch > p.InitialWindow {
		return fmt.Errorf("window update batch must be between 1 and the initial window")
	}
	if p.AcceptBacklog <= 0 {
		return fmt.Errorf("accept backlog must be positive")
	}
	return nil
}
