package compliance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	// scamDBRemoteURL is the ScamSniffer combined blacklist.
	scamDBRemoteURL = "https://raw.githubusercontent.com/scamsniffer/scam-database/main/blacklist/combined.json"

	// scamDBFetchTimeout is the HTTP timeout for fetching the remote database.
	scamDBFetchTimeout = 30 * time.Second

	// scamDBRefreshInterval is how often the background goroutine refreshes.
	scamDBRefreshInterval = 6 * time.Hour
)

// ScamDB holds a set of known scam/phishing wallet addresses sourced from
// the ScamSniffer database. All addresses are stored lowercase.
type ScamDB struct {
	mu    sync.RWMutex
	addrs map[string]bool
}

// NewScamDB creates a ScamDB pre-loaded with a hardcoded default set of the
// most prolific scam addresses from ScamSniffer.
func NewScamDB() *ScamDB {
	db := &ScamDB{addrs: make(map[string]bool, len(defaultScamAddresses))}
	for _, addr := range defaultScamAddresses {
		db.addrs[addr] = true
	}
	log.Info().Int("count", len(db.addrs)).Msg("scamdb: loaded default scam addresses")
	return db
}

// Check returns whether the given address is in the scam database.
func (db *ScamDB) Check(address string) (isScam bool, source string) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if db.addrs[strings.ToLower(address)] {
		return true, "scamsniffer"
	}
	return false, ""
}

// Count returns the number of addresses in the database.
func (db *ScamDB) Count() int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return len(db.addrs)
}

// RefreshFromRemote fetches the latest ScamSniffer combined.json and merges
// all addresses into the local set. Existing addresses are preserved.
func (db *ScamDB) RefreshFromRemote(ctx context.Context) error {
	client := &http.Client{Timeout: scamDBFetchTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, scamDBRemoteURL, nil)
	if err != nil {
		return fmt.Errorf("scamdb: build request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("scamdb: fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("scamdb: unexpected status %d", resp.StatusCode)
	}

	// Format: {"domain": ["0xaddr1", "0xaddr2"], ...}
	var data map[string][]string
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fmt.Errorf("scamdb: decode: %w", err)
	}

	// Collect unique addresses.
	fresh := make(map[string]bool, 1024)
	for _, addrs := range data {
		for _, addr := range addrs {
			fresh[strings.ToLower(addr)] = true
		}
	}

	db.mu.Lock()
	for addr := range fresh {
		db.addrs[addr] = true
	}
	total := len(db.addrs)
	db.mu.Unlock()

	log.Info().Int("fetched", len(fresh)).Int("total", total).Msg("scamdb: refreshed from remote")
	return nil
}

// StartBackgroundRefresh spawns a goroutine that refreshes the scam DB from
// GitHub every 6 hours. It stops when ctx is cancelled.
func (db *ScamDB) StartBackgroundRefresh(ctx context.Context) {
	go func() {
		// Initial fetch on startup.
		if err := db.RefreshFromRemote(ctx); err != nil {
			log.Error().Err(err).Msg("scamdb: initial refresh failed, using defaults")
		}

		ticker := time.NewTicker(scamDBRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := db.RefreshFromRemote(ctx); err != nil {
					log.Error().Err(err).Msg("scamdb: periodic refresh failed")
				}
			}
		}
	}()
}

// defaultScamAddresses is the top ~150 most prolific scam wallet addresses
// from the ScamSniffer database, ranked by number of phishing domains using
// each address. All lowercase.
var defaultScamAddresses = []string{
	"0xc75269b342c1b7f4cbb82e80a7986878ac0f545b",
	"0x34f3f4ba979e177a517970e014250cab61a80529",
	"0xb42e42c80f6af7b6afd8877f4853f0bbc0eb3a43",
	"0xff92104ffa62db76aa7fc9ec97442dacfe05e99c",
	"0xf3cc7bf7b8cf56bcbe500ff0af063383f97108f1",
	"0xa31573be292bd03d36db137b6c2ab6eaa3d5e572",
	"0x082666f545ec0c9aa60f2f02d175da4955b954ae",
	"0x3569563839c4e308f09122126b4fad3ecaa99999",
	"0x85b9ec2787d47b2cfc21445d19aba062cc3ce923",
	"0x9b2754a30545d0f66ae90c1b44e686bf98595963",
	"0x5f42269e5f625c85d376a13b76692f1a81b38d8a",
	"0x38313c0668c897b6d591a69e0a9fdc9305e67b55",
	"0xdcdcf1f321ae47eebbe94f695483529d03e52395",
	"0xfe3179f5e373f266ed0e438b8c79769697436bad",
	"0x8a25cf46d8170d4b3edf31b78ecd12d43994caab",
	"0x627fe14e789380067688058c1f23d16b0690eb2c",
	"0x7d8904598e5ec80ffb241349767a1840bd35d51d",
	"0xe5e5a2926b282a9b19015b212cf2974bdf5bba08",
	"0xccfd29158dfc8320cdd4b7ce7a3373805cfdda6e",
	"0x4c0ce7248e8a18d54009b8d2ba982d1b0ca4ea49",
	"0x0d524a5b52737c0a02880d5e84f7d20b8d66bfba",
	"0xd23e368689ad3faba1817d220a56068e9527b600",
	"0x9afdbc669f83f2eb81f5cb3c12d26454cb9c0e27",
	"0xac8f8a4f693c6bef022d31936fb75e12172efaa3",
	"0x1099ac30cdc4159b7a8c4acca27aa14628d5a93f",
	"0xbc1a7736c12dc64632fd51f16a5271a0bc202acd",
	"0x80b750e36eca41f0d5b96aefa875de133725044e",
	"0x7bc415201e3a17d6624b586aeaefd9e2a0b1b8b3",
	"0x55c3f34c992cbac8d966609773fbc441dddd9c45",
	"0xe9e6049d4a1eeb306690bb6ec18bab3215c25751",
	"0x19a72ea327b37e3650e1092abd724c723172962c",
	"0x85f1eea6c1159f68742c6c959a6a6d15daaf2471",
	"0xc3205702a9e793d6a23d1bb19f5531632ef0b768",
	"0xc4b0e6cf56e76e831e13d197705b4ce05c472ddc",
	"0x41126e19981eb1bbfe92b1d1cc4fe7104b044ab2",
	"0x0e0bb0cc029c221277329552403c976e1bd176d2",
	"0x9864d21466010ffac0305ba9e6039976aab40995",
	"0x3da02e1f29bcbed185eca0d3299efd46e6e7e155",
	"0x003f13d95ccbd7b8778ca66be31adf71add79ee6",
	"0x2ad0875e63116e96eec7c4d38a726457b628dd68",
	"0x4d6a07ad7e7870e5d0fe4bb3d0a6a4c7d5e15f56",
	"0x76c4bf187f40cfb56ee0a5e5f26fa1f804827929",
	"0x463224c0af60e2e47f9c6320f337ba94e809ddb3",
	"0xc33d8f9b4859b7ce8267c5f6eb1526b16ecf0863",
	"0x3da336601df9dccb0203f57922c919649a8981d8",
	"0x6fe2ee963643ac7e480aacafefddb3683e38fcf9",
	"0x9d3829eaac81a70f62ee3c0b5cc3b3f72f5bd1b3",
	"0xc5602b6d8f56a145779495d0e21685d5d99c4907",
	"0x29a470bb2dd172340ec113ebdcc2f3ef762ab0a7",
	"0xd2089ff4e050a29e85fb5a447f83628e2a697555",
	"0x61018bdce873eb4a325b35471443bf4805f0da81",
	"0xc33e89982b55d5cbdc3c651786ed243ab2e635b6",
	"0x0aa7f992dfb485cf9c4fbe9688f1ecdf9e0a15f9",
	"0x41d2c6e78f53cad7dc630c53d7f7204af1aae322",
	"0x7b92fb979ae44b646121e5ec07abf82a7022883d",
	"0x2f71039339f18d2f16cccc1292ae6279568d1b68",
	"0x787450d78225fb16300cb9a10a1af648c2abd115",
	"0x3b37e90a131d8c531903bf16d005254345677d04",
	"0x8a8a2436fc920e6c73c3a9e9a00b8d937812ee0d",
	"0xf9e56d2e507cba5d71a2dbec06a03d2700e09a12",
	"0xc74cdbbdfcfe02711dde8e7faca0f74186932414",
	"0x433d46d0c76841ef7fe48aee4399801ada3cadda",
	"0x865acd696c73d2a26ef4ff1a974c1fad6df3fb8d",
	"0x4afe18cbf1f12bbad712fea55af296a8d54b6c27",
	"0x28c8d8709a341713bddb3f555de4ef4448e064dd",
	"0xda3ffd3fbaa65e04eab0511dc8064effe86f1f85",
	"0x818af6a967fb329c04abc9d9e2301483cc268636",
	"0x3ecd1df0c3a85d8ef8cfd48f163306ae0a74b4d9",
	"0x6cd72e17f2086e123825e13b4d0a78d61916d75b",
	"0xac7be03e3a2c30bcc7eec7f10146d41581f4fce2",
	"0x667200d20087357f77a5b5e5805575285ee27571",
	"0xc1f908863feca9f0b352d31606173d13fa113134",
	"0x685e7db34eae3320fe1673b1507c12b6a6a19324",
	"0x41833dc56eda6e81f70db9b3931da2c97cd63287",
	"0x41e8a9188136cf26bda9bd33acbfd0bc808a025b",
	"0x3209512d3177a145447184f137a73755673f8340",
	"0x61c150ad33833f19a08a0552cf6726966f336bbd",
	"0xd075bf8193196c68cec8e5ee487c938b2ceeb401",
	"0x86544f7e7a90d05048469d209d6c3d1009e8c3e0",
	"0x727b6866bdba3921bda2757c42956a02c71f8d7e",
	"0xcc3ee5ab82949e84132e790e63a4105b9b599823",
	"0x656f840a06c769f44b7d39a1df04897af052795e",
	"0xa4674d608d38822765af8bf76562f6457a2c4621",
	"0xa0e7959da78b1188604a7f0a3fbaa7150fe2a490",
	"0xc9ca30b4b7dc3f2d9f3a618ba411c049d2f5fce6",
	"0xa2e01fd8f960f01b99d7b0c63c65f9f3cd0629cc",
	"0x11ceb3aab35e2510d7eee3728468cfea7c39bee6",
	"0xb8659dc05a5668e502009513e0bfa5cb9bf1579c",
	"0x710b5bb4552f20524232ae3e2467a6dc74b21982",
	"0xd10471a6f98be2fa63e8678b1067ee10b151ae22",
	"0xdbdd8d8340f59e30e05b3cb3fb96a0b79f4a597c",
	"0x7b732e6eb24dc885db7ff417478dea2c1d4d64b2",
	"0xb576859a3b150bae6a748d8fc9a82ebfc1389386",
	"0xc28d5d922eb85c51b114f35242216ae792f29f16",
	"0xad445ca90e15d587b1b6182dd47334a5990026b4",
	"0xaa01c987952db328de3aee15a06f38c7981ae8fb",
	"0xeee495d9fa356175623e227546cbe46dffdaf6f2",
	"0x7b7a7f26db00692ed0bf7cc5aa842f1a9706e296",
	"0x40c8590f2d2f543689b42f9a1f1872cb7461aa0d",
	"0x37a51fd428bc37d5a3be6c43b32446c602c1983b",
	"0xa342a1904b6fc4d659b7dae30fca2e87ef2f154a",
	"0xd67580f10d3740342113c2451be45a557ebab42e",
	"0x75134927e043e8b6e628ecb91891448773662cb1",
	"0xbdab7c50aa43220d15a11006375c728731d449b1",
	"0x8239fc719f4fb9184eb3d1c50f79a9b52b23ed1b",
	"0xffb2b004d89983d55dda6beb927b7d43fbfadf7f",
	"0xcab2fd99e002fb29e4ac6aacda371971de1f6bf8",
	"0xd9d49546c9353f6ebfd6ee28c6db506ca0645381",
	"0x112893f553d48a3917475fec16867a063d15e89d",
	"0xeab5fd5c3c781514c1264730fcfd8420e6df6e8e",
	"0xb1f6ca0379c32804fd17ad573db3078922aaee68",
	"0x7fe421a8fc84d3c58548e32f5d5fb7a7d5757e55",
	"0xd3ed4e39917805550a3fe4bfbc02455fee945e9d",
	"0x6ae9a3937e9df8bff944ec0307e7d4748533c5f7",
	"0xe10da2c1edac5e2061b19b3504a7608142d96f59",
	"0x1d5071048370df50839c8879cdf5144ace4b3b3b",
	"0x6fd99b85200ff44ab2c667532d4f7166ca0aa230",
	"0x935d9c4da96c55bedd432f0fa73f0958aed3d51b",
	"0x21541e17c6d0b12b80fad974911a4eb13a84031f",
	"0x398e98b7c19db2f5df086eb4f83624146aa1ab53",
	"0xd307e99b79c4cb290a9d4583ac3b31df39e06ffb",
	"0x10fd497e61b1aa7055ece54b3f06eeec511caa47",
	"0xbf3790474b0c2aad0f4783c68323a1c9011de414",
	"0xbac0ffc1e838555504215c83f497627c921886a8",
	"0xe9b01a715c17080a4fd8182cfc2e77595f7c62ce",
	"0x913f9b4d7ffeec32d885932007711ab0c8710ae7",
	"0x9350dc887ee1c4ae0b3b0404666020008393f457",
	"0xbd4fee42893e8bac47541dc2b5229d7139186666",
	"0xe49a090d35566ed79c3a2c665476dcaca57ac01b",
	"0xfa510abde79c3e10b0dfe1b071726a2dede3a536",
	"0x86d021e4b742a7abcb6b12321b5f59fb419a167c",
	"0x8dd465df3d1d4e0a4ae978fa7fb30eb558ab8714",
	"0x0cec7843b9c2b125d837d87f0c9197724a94bf42",
	"0xfb98172ae8431830fbcb9fe7380cf88985c824a4",
	"0x403ad631954bb71f4d5476cd61f395f631bc8824",
	"0x08bc4c07a72fd3bf26565516b5d6de1fee715885",
	"0x8987ed1907f3048e799d4a474d77261cbc5f9795",
	"0x1890314ac916fc0ddb8d5f38d9ddc6d6eb51d12e",
	"0x36d5dc95503db75e8c014d2f0706f6aa1041fb40",
	"0x05eb198b38bbb89c22247010672341572479cb16",
	"0x09faf279be240485fc8d77b271deca50b52e8899",
	"0x54c3bdccf63c29f725dd2d9804c5655c863587ab",
	"0xce064d7145a30c5aea11e64b77723bccf12b779e",
	"0xc40a8a6763969e88c8bf58a6e7a5adc61b8ebe11",
	"0x7d60695116f5608626e8f95d6de923a7db4bf634",
	"0x919f08ee29b8a6b681e44b785f3626ec3c249d19",
	"0xcea0f1fad75e62dcf89ab18d6566482916baa65c",
	"0x651537952aded446fdee91124679d8562bdd059f",
	"0xdacea04a43eaf3c5a6f171cdfbd9ae7447faf325",
	"0x6f777797d9c60ae334fc4561b0c931fa9f442395",
	"0xd7cf8f30c1f26a392cfee289ac4a2f114ee8ca2b",
	"0x7e92b316d18459e859f33d345d41db6c6d90f615",
	"0xa10df3f212e8480db5fff956d46945b25c762045",
}
