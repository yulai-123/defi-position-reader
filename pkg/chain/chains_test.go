package chain

import "testing"

func TestDefaultChainsReturnsCopy(t *testing.T) {
	t.Parallel()

	chains := DefaultChains()
	if len(chains) != 3 {
		t.Fatalf("expected 3 default chains, got %d", len(chains))
	}

	chains[0].Name = "changed"
	again := DefaultChains()
	if again[0].Name == "changed" {
		t.Fatal("DefaultChains should return a copy")
	}
}

func TestChainLookup(t *testing.T) {
	t.Parallel()

	ethereum, ok := ByName(" Ethereum ")
	if !ok {
		t.Fatal("expected ethereum chain")
	}
	if ethereum.ID != EthereumChainID {
		t.Fatalf("unexpected ethereum chain id: %d", ethereum.ID)
	}

	base, ok := ByID(BaseChainID)
	if !ok || base.Name != "base" {
		t.Fatalf("unexpected base lookup: %#v %t", base, ok)
	}

	if _, ok := ByName("missing"); ok {
		t.Fatal("missing chain should not be found")
	}
	if _, ok := ByID(999999); ok {
		t.Fatal("missing chain id should not be found")
	}
}
