package main

import "testing"

func TestOfflineOrderRequestHashStableAndPayloadSensitive(t *testing.T) {
	base := createPOSOrderRequest{ServiceType: "walk_in", Label: "Takeaway", Lines: []saleQuoteInputLine{{ProductID: "00000000-0000-4000-8000-000000000001", Quantity: 2}}}
	first, err := offlineOrderRequestHash(base)
	if err != nil {
		t.Fatalf("hash request: %v", err)
	}
	second, err := offlineOrderRequestHash(base)
	if err != nil {
		t.Fatalf("hash replay: %v", err)
	}
	if first != second {
		t.Fatal("identical offline order payloads must have identical hashes")
	}
	changed := base
	changed.Lines = []saleQuoteInputLine{{ProductID: "00000000-0000-4000-8000-000000000001", Quantity: 3}}
	third, err := offlineOrderRequestHash(changed)
	if err != nil {
		t.Fatalf("hash changed request: %v", err)
	}
	if first == third {
		t.Fatal("different offline order payloads must not share a hash")
	}
}
