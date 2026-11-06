package semantics

// Engine stats the sheet reads (v0). rules/poe1 registers them; their formulas decide how,
// for example, life_per_level and strength reach life.
const (
	Strength         = "strength"
	Dexterity        = "dexterity"
	Intelligence     = "intelligence"
	Life             = "life"
	LifePerLevel     = "life_per_level"
	Mana             = "mana"
	ManaPerLevel     = "mana_per_level"
	EnergyShield     = "energy_shield"
	Evasion          = "evasion"
	Armour           = "armour"
	FireRes          = "fire_resistance"
	ColdRes          = "cold_resistance"
	LightningRes     = "lightning_resistance"
	ChaosRes         = "chaos_resistance"
	ElementalRes     = "elemental_resistance"
	FireResMax       = "fire_resistance_max"
	ColdResMax       = "cold_resistance_max"
	LightningResMax  = "lightning_resistance_max"
	ChaosResMax      = "chaos_resistance_max"
	ElementalResMax  = "elemental_resistance_max"
	LifeRegen        = "life_regen"
	LifeRegenPercent = "life_regen_percent"
	ManaRegen        = "mana_regen"
	ManaRegenPercent = "mana_regen_percent"
)

// EngineStats lists every engine stat a table may name.
var EngineStats = []string{
	Strength, Dexterity, Intelligence, Life, LifePerLevel, Mana, ManaPerLevel, EnergyShield,
	Evasion, Armour, FireRes, ColdRes, LightningRes, ChaosRes, ElementalRes, FireResMax,
	ColdResMax, LightningResMax, ChaosResMax, ElementalResMax, LifeRegen, LifeRegenPercent,
	ManaRegen, ManaRegenPercent,
}

// pobNames maps PoB modifier names to engine stats. This is the one place PoB's names live.
// An empty list marks a name the sheet does not need: StrDex and the like are PoB's
// bookkeeping for "per attribute" lookups, emitted next to the real attribute modifiers.
var pobNames = map[string][]string{
	"Str":                {Strength},
	"Dex":                {Dexterity},
	"Int":                {Intelligence},
	"StrDex":             {},
	"StrInt":             {},
	"DexInt":             {},
	"All":                {},
	"Life":               {Life},
	"Mana":               {Mana},
	"EnergyShield":       {EnergyShield},
	"Evasion":            {Evasion},
	"Armour":             {Armour},
	"ArmourAndEvasion":   {Armour, Evasion},
	"FireResist":         {FireRes},
	"ColdResist":         {ColdRes},
	"LightningResist":    {LightningRes},
	"ChaosResist":        {ChaosRes},
	"ElementalResist":    {ElementalRes},
	"FireResistMax":      {FireResMax},
	"ColdResistMax":      {ColdResMax},
	"LightningResistMax": {LightningResMax},
	"ChaosResistMax":     {ChaosResMax},
	"ElementalResistMax": {ElementalResMax},
	"LifeRegen":          {LifeRegen},
	"LifeRegenPercent":   {LifeRegenPercent},
	"ManaRegen":          {ManaRegen},
	"ManaRegenPercent":   {ManaRegenPercent},
}

// pobTypes maps PoB modifier types to ops.
var pobTypes = map[string]Op{
	"BASE":     OpBase,
	"INC":      OpInc,
	"MORE":     OpMore,
	"OVERRIDE": OpOverride,
	"FLAG":     OpFlag,
}
