package MEVless

type Config struct {
	PackNumber    uint64 `toml:"pack_number"`
	Addr          string `toml:"addr"`
	AdvanceCharge uint64 `toml:"advance_charge"`
}

func DefaultCfg() *Config {
	return &Config{
		PackNumber:    10000,
		Addr:          "localhost:9071",
		AdvanceCharge: 1000,
	}
}
