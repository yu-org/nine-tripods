package MEVless

type Config struct {
	PackNumber    uint64 `toml:"pack_number"`
	Port          string `toml:"port"`
	AdvanceCharge uint64 `toml:"advance_charge"`
}
