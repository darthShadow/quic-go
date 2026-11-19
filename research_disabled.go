//go:build !shunpike_research

package quic

func researchGapLimit() int         { return 0 }
func observeGapCount(uint64)        {}
func recordGapAbort(byteInterval)   {}
func recordReceiveQueueDrop()       {}
func recordMTUFallback(int, string) {}
