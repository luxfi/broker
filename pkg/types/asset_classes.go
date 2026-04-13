// Canonical asset class vocabulary. Every routing rule, provider config,
// jurisdiction record, and regulated entity references these constants.
// There is exactly one way to name an asset class.
package types

// AssetClass is a typed string for compile-time safety.
type AssetClass string

const (
	// US asset classes
	AssetClassUSEquity      AssetClass = "us_equity"
	AssetClassUSCrypto      AssetClass = "us_crypto"
	AssetClassUSOptions     AssetClass = "us_options"
	AssetClassUSFixedIncome AssetClass = "us_fixed_income"

	// UK asset classes (FCA)
	AssetClassUKEquity AssetClass = "uk_equity"
	AssetClassUKCrypto AssetClass = "uk_crypto"
	AssetClassUKCFDs   AssetClass = "uk_cfds"
	AssetClassUKFI     AssetClass = "uk_fi"

	// India asset classes (SEBI)
	AssetClassINEquity    AssetClass = "in_equity"
	AssetClassINCommodity AssetClass = "in_commodity"
	AssetClassINFnO       AssetClass = "in_fno"

	// Singapore asset classes (MAS)
	AssetClassSGEquity AssetClass = "sg_equity"
	AssetClassSGCrypto AssetClass = "sg_crypto"
	AssetClassSGREITs  AssetClass = "sg_reits"

	// Australia asset classes (ASIC)
	AssetClassAUEquity AssetClass = "au_equity"
	AssetClassAUETF    AssetClass = "au_etf"
	AssetClassAUCrypto AssetClass = "au_crypto"

	// Canada asset classes (CIRO)
	AssetClassCAEquity AssetClass = "ca_equity"
	AssetClassCAETF    AssetClass = "ca_etf"
	AssetClassCACrypto AssetClass = "ca_crypto"

	// Brazil asset classes (CVM)
	AssetClassBREquity  AssetClass = "br_equity"
	AssetClassBROptions AssetClass = "br_options"
	AssetClassBRFIIs    AssetClass = "br_fiis"
	AssetClassBRCrypto  AssetClass = "br_crypto"

	// Switzerland asset classes (FINMA)
	AssetClassCHEquity      AssetClass = "ch_equity"
	AssetClassCHDLTSecurity AssetClass = "ch_dlt_security"
	AssetClassCHCrypto      AssetClass = "ch_crypto"
	AssetClassCHFI          AssetClass = "ch_fi"

	// UAE asset classes (SCA/DFSA/VARA/FSRA)
	AssetClassAEEquity            AssetClass = "ae_equity"
	AssetClassAECrypto            AssetClass = "ae_crypto"
	AssetClassAEFI                AssetClass = "ae_fi"
	AssetClassAETokenizedSecurity AssetClass = "ae_tokenized_security"
	AssetClassAEFunds             AssetClass = "ae_funds"

	// EU asset classes (MiCA-harmonized)
	AssetClassEUEquity      AssetClass = "eu_equity"
	AssetClassEUMiFIDFI     AssetClass = "eu_mifid_fi"
	AssetClassEUUCITS       AssetClass = "eu_ucits"
	AssetClassEUAIF         AssetClass = "eu_aif"
	AssetClassEUART         AssetClass = "eu_art"
	AssetClassEUEMT         AssetClass = "eu_emt"
	AssetClassEUCAS         AssetClass = "eu_cas"
	AssetClassEUDLTSecurity AssetClass = "eu_dlt_security"

	// Wildcard — matches any asset class in routing rules
	AssetClassWildcard AssetClass = "*"
)

// AllAssetClasses returns every defined asset class (excluding wildcard).
func AllAssetClasses() []AssetClass {
	return []AssetClass{
		AssetClassUSEquity, AssetClassUSCrypto, AssetClassUSOptions, AssetClassUSFixedIncome,
		AssetClassUKEquity, AssetClassUKCrypto, AssetClassUKCFDs, AssetClassUKFI,
		AssetClassINEquity, AssetClassINCommodity, AssetClassINFnO,
		AssetClassSGEquity, AssetClassSGCrypto, AssetClassSGREITs,
		AssetClassAUEquity, AssetClassAUETF, AssetClassAUCrypto,
		AssetClassCAEquity, AssetClassCAETF, AssetClassCACrypto,
		AssetClassBREquity, AssetClassBROptions, AssetClassBRFIIs, AssetClassBRCrypto,
		AssetClassCHEquity, AssetClassCHDLTSecurity, AssetClassCHCrypto, AssetClassCHFI,
		AssetClassAEEquity, AssetClassAECrypto, AssetClassAEFI, AssetClassAETokenizedSecurity, AssetClassAEFunds,
		AssetClassEUEquity, AssetClassEUMiFIDFI, AssetClassEUUCITS, AssetClassEUAIF,
		AssetClassEUART, AssetClassEUEMT, AssetClassEUCAS, AssetClassEUDLTSecurity,
	}
}

// ValidAssetClass returns true if s is a known asset class or wildcard.
func ValidAssetClass(s string) bool {
	if s == string(AssetClassWildcard) {
		return true
	}
	for _, ac := range AllAssetClasses() {
		if string(ac) == s {
			return true
		}
	}
	return false
}

// AssetClassStrings returns all asset class string values (for Base select fields).
func AssetClassStrings() []string {
	all := AllAssetClasses()
	out := make([]string, len(all)+1)
	for i, ac := range all {
		out[i] = string(ac)
	}
	out[len(all)] = string(AssetClassWildcard)
	return out
}
