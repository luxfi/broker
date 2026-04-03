package compliance

import "strings"

// ofacSanctionedAddresses contains crypto wallet addresses from the OFAC SDN
// list. All keys are stored lowercase for case-insensitive lookup.
var ofacSanctionedAddresses = func() map[string]string {
	// source -> address (lowercase)
	raw := map[string]string{
		// Tornado Cash
		"0x8589427373D6D84E98730D7795D8f6f8731FDA16": "tornado_cash",
		"0xd90e2f925DA726b50C4Ed8D0Fb90Ad053324F31b": "tornado_cash",
		"0xDD4c48C0B24039969fC16D1cdF626eaB821d3384": "tornado_cash",
		"0x722122dF12D4e14e13Ac3b6895a86e84145b6967": "tornado_cash",
		"0xd4B88Df4D29F5CedD6857912842cff3b20C8Cfa3": "tornado_cash",
		"0x910Cbd523D972eb0a6f4cAe4618aD62622b39DbF": "tornado_cash",
		"0xA160cdAB225685dA1d56aa342Ad8841c3b53f291": "tornado_cash",
		"0xFD8610d20aA15b7B2E3Be39B396a1bC3516c7144": "tornado_cash",
		"0xF60dD140cFf0706bAE9Cd734Ac3683f2dB7085c5": "tornado_cash",
		"0x23773E65ed146A459791799d01336DB287f25334": "tornado_cash",
		"0x12D66f87A04A9E220743712cE6d9bB1B5616B8Fc": "tornado_cash",
		"0x47CE0C6eD5B0Ce3d3A51fdb1C52DC66a7c3c2936": "tornado_cash",
		"0xD21be7248e0197Ee08E0c20D4a398a3aA47b7857": "tornado_cash",
		"0x178169B423a011fff22B9e3F3abeA13414dDD0F1": "tornado_cash",
		"0x610B717796ad172B316836AC95a2ffad065CeaB4": "tornado_cash",
		"0xbB93e510BbCD0B7beb5A853875f9eC60275CF498": "tornado_cash",

		// Lazarus Group (DPRK)
		"0x098B716B8Aaf21512996dC57EB0615e2383E2f96": "lazarus_group",
		"0xa0e1c89Ef1a489c9C7dE96311eD5Ce5D32c20E4B": "lazarus_group",
		"0x3Cffd56B47B7b41c56258D9C7731ABaDc360E460": "lazarus_group",
		"0x53b6936513e738f44FB50d2b9476730C0Ab3Bfc1": "lazarus_group",
		"0x35fB6f6DB4fb05e6A4cE86f2C93270f0461b11f3": "lazarus_group",
		"0xF7B31119c2682c88d88D455dBb9d5932c65Cf1bE": "lazarus_group",
		"0x3AD9dB589d201A710Ed237c829c7860Ba86510Fc": "lazarus_group",
		"0xC8487fCAe8D1547C96DAe3e5b3b4c3F36EC20132": "lazarus_group",

		// Garantex
		"0x6Bf694a291DF3FeC1f7e69701E3ab6c592435Ae7": "garantex",

		// Blender.io
		"0x36dd7e6e2A8b3E1dC5e69B7b5e1B7C8Ca5e8b6E3": "blender_io",
		"0xBA214C1c1928a32Bffe790263E38B4Af9bFCD659": "blender_io",
		"0xb1C8094B234DcE6e03f10a5b673c1d8C69739A00": "blender_io",
		"0x527653eA119F3E6a1F5BD18fbF4714081D7B31ce": "blender_io",
		"0x58E8dCC13BE9780fC42E8723D8EaD4CF46943dF2": "blender_io",

		// Chatex
		"0x6F1cA141A28907F78Ebaa64f83D078e664F2bF4e": "chatex",
		"0x6aCDFBA02D390b97Ac2b2d42A63E85293BCc160e": "chatex",

		// Suex
		"0x2f389cE8bD8ff92De3402FFCe4691d17fC4f6535": "suex",

		// Hydra Market
		"0x48549A34AE37b12F6a30566245176994e17C6b4A": "hydra_market",

		// Sinbad
		"0x723B78e67497E85279CB204544566F4dC5d2acA0": "sinbad",
		"0xA7e5d5A720f06526557c513402f2e6B5fA20b008": "sinbad",

		// Additional OFAC-listed (individual sanctions)
		"0x7F367cC41522cE07553e823bf3be79A889DEbe1B": "ofac_individual",
		"0xd882cFc20F52f2599D84b8e8D58C7FB62cfE344b": "ofac_individual",
		"0x7Db418b5D567A4e0E8c59Ad71BE1FcE48f3E6107": "ofac_individual",
		"0x72a5843cc08275C8171E582972Aa4fDa8C397B2A": "ofac_individual",
		"0x7F19720A857F834696350e4fF838329a3a80b29C": "ofac_individual",
		"0x1da5821544e25c636c1417Ba96Ade4Cf6D2f9B5A": "ofac_individual",
		"0x9F4cda013E354b8fC285BF4b9A60460cEe7f7Ea9": "ofac_individual",
	}

	m := make(map[string]string, len(raw))
	for addr, source := range raw {
		m[strings.ToLower(addr)] = source
	}
	return m
}()

// isOFACSanctioned checks if an address is on the OFAC SDN list.
// Returns the sanctions program name if found, empty string otherwise.
func isOFACSanctioned(address string) (source string, sanctioned bool) {
	source, sanctioned = ofacSanctionedAddresses[strings.ToLower(address)]
	return
}
