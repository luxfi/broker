package types

import "testing"

func TestValidAssetClass(t *testing.T) {
	valid := []string{
		"us_equity", "us_crypto", "us_options", "us_fixed_income",
		"uk_equity", "uk_crypto", "uk_cfds", "uk_fi",
		"in_equity", "in_commodity", "in_fno",
		"sg_equity", "sg_crypto", "sg_reits",
		"au_equity", "au_etf", "au_crypto",
		"ca_equity", "ca_etf", "ca_crypto",
		"br_equity", "br_options", "br_fiis", "br_crypto",
		"*",
	}
	for _, v := range valid {
		if !ValidAssetClass(v) {
			t.Errorf("ValidAssetClass(%q) = false, want true", v)
		}
	}
}

func TestInvalidAssetClass(t *testing.T) {
	invalid := []string{"", "equity", "stock", "US_EQUITY", "us-equity", "crypto"}
	for _, v := range invalid {
		if ValidAssetClass(v) {
			t.Errorf("ValidAssetClass(%q) = true, want false", v)
		}
	}
}

func TestAllAssetClassesNoDuplicates(t *testing.T) {
	seen := map[AssetClass]bool{}
	for _, ac := range AllAssetClasses() {
		if seen[ac] {
			t.Fatalf("duplicate asset class: %s", ac)
		}
		seen[ac] = true
	}
}

func TestAssetClassStringsIncludesWildcard(t *testing.T) {
	strs := AssetClassStrings()
	found := false
	for _, s := range strs {
		if s == "*" {
			found = true
		}
	}
	if !found {
		t.Fatal("AssetClassStrings() missing wildcard")
	}
	if len(strs) != len(AllAssetClasses())+1 {
		t.Fatalf("AssetClassStrings() len = %d, want %d", len(strs), len(AllAssetClasses())+1)
	}
}
