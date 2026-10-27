package clock

// Wire model: this file is converted to TypeScript by tygo (frontend/src/widgets/clock/model.gen.ts).

type Data struct {
	// Timezone is an IANA name; empty means the device's timezone.
	Timezone         string `json:"timezone"`
	UTCOffsetSeconds int    `json:"utcOffsetSeconds"`
}

// Settings of a core.clock widget (the YAML under "settings:").
type Settings struct {
	Timezone    string `json:"timezone,omitempty"` // IANA name; empty = the device's timezone
	ShowSeconds bool   `json:"showSeconds,omitempty"`
	Hour12      bool   `json:"hour12,omitempty"` // 12-hour clock with am/pm
	Label       string `json:"label,omitempty"`  // small caption above the time
}
