package core

import "testing"

func TestProtocolDescriptorSupportsChain(t *testing.T) {
	t.Parallel()

	descriptor := ProtocolDescriptor{SupportedChains: []int64{1, 8453}}
	if !descriptor.SupportsChain(8453) {
		t.Fatal("expected descriptor to support Base")
	}
	if descriptor.SupportsChain(42161) {
		t.Fatal("did not expect descriptor to support Arbitrum")
	}
}
