package MEVless

type Config struct {
	PackNumber    uint64 `toml:"pack_number"`
	AdvanceCharge uint64 `toml:"advance_charge"`
}
