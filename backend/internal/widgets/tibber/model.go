package tibber

// Wire model: this file is converted to TypeScript by tygo (frontend/src/widgets/tibber/model.gen.ts).
// Keep it free of anything that is not part of the data sent to the browser.

// Data is the widget's data model (mirrored in frontend/src/widgets/tibber).
type Data struct {
	Label    string `json:"label"`    // display name; defaults to the Tibber home's name
	Currency string `json:"currency"` // ISO code, e.g. EUR, NOK, SEK
	Timezone string `json:"timezone"` // IANA name the day boundaries refer to
	StepSec  int    `json:"stepSec"`  // length of one price slot in seconds (900)
	Slots    []Slot `json:"slots"`    // published prices from the start of today on, oldest first
}

// Slot is the price of one interval per kWh in the main currency unit (EUR, not ct).
type Slot struct {
	T     int64   `json:"t"` // start of the interval, unix seconds
	Price float64 `json:"price"`
}

// Settings of a tibber.prices widget. The OAuth client lives in the connection.
type Settings struct {
	Home  string `json:"home,omitempty"`  // Tibber home id or name; defaults to the first home
	Label string `json:"label,omitempty"` // display name; defaults to the home's name
}
