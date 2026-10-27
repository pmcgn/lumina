package weather

// Wire model: this file is converted to TypeScript by tygo (frontend/src/widgets/weather/model.gen.ts).
// Keep it free of anything that is not part of the data sent to the browser.

// Data is the widget's data model (mirrored in frontend/src/widgets/weather).
type Data struct {
	Location string  `json:"location"`
	Timezone string  `json:"timezone"` // IANA name of the location
	Current  Current `json:"current"`
	Hourly   []Hour  `json:"hourly"` // next 24 hours
	Daily    []Day   `json:"daily"`
}

type Current struct {
	TempC      float64   `json:"tempC"`
	FeelsLikeC float64   `json:"feelsLikeC"`
	Humidity   int       `json:"humidity"`
	PrecipMM   float64   `json:"precipMm"`
	WindKmh    float64   `json:"windKmh"`
	WindDir    int       `json:"windDirDeg"`
	Condition  Condition `json:"condition"`
	IsDay      bool      `json:"isDay"`
}

type Hour struct {
	T          int64     `json:"t"` // unix seconds
	TempC      float64   `json:"tempC"`
	PrecipProb int       `json:"precipProb"`
	Condition  Condition `json:"condition"`
}

type Day struct {
	Date       string    `json:"date"` // YYYY-MM-DD in the location's timezone
	MinC       float64   `json:"minC"`
	MaxC       float64   `json:"maxC"`
	PrecipProb int       `json:"precipProb"`
	Condition  Condition `json:"condition"`
	Sunrise    int64     `json:"sunrise"` // unix seconds
	Sunset     int64     `json:"sunset"`
}

// Condition is a coarse, translatable weather state.
type Condition string

const (
	ConditionClear        Condition = "clear"
	ConditionPartlyCloudy Condition = "partly-cloudy"
	ConditionCloudy       Condition = "cloudy"
	ConditionFog          Condition = "fog"
	ConditionDrizzle      Condition = "drizzle"
	ConditionRain         Condition = "rain"
	ConditionShowers      Condition = "showers"
	ConditionFreezingRain Condition = "freezing-rain"
	ConditionSnow         Condition = "snow"
	ConditionSnowShowers  Condition = "snow-showers"
	ConditionThunderstorm Condition = "thunderstorm"
)

// Settings of a weather.forecast widget. Give either latitude and longitude, or a location name.
type Settings struct {
	Latitude  *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
	Location  string   `json:"location,omitempty"` // place name, looked up once (Open-Meteo geocoding)
	Label     string   `json:"label,omitempty"`    // display name; defaults to the location name
}
