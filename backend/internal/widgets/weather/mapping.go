package weather

import (
	"errors"
	"fmt"
	"time"
)

// conditionFromWMO maps WMO weather interpretation codes (as used by Open-Meteo).
func conditionFromWMO(code int) Condition {
	switch code {
	case 0, 1:
		return ConditionClear
	case 2:
		return ConditionPartlyCloudy
	case 3:
		return ConditionCloudy
	case 45, 48:
		return ConditionFog
	case 51, 53, 55:
		return ConditionDrizzle
	case 56, 57, 66, 67:
		return ConditionFreezingRain
	case 61, 63, 65:
		return ConditionRain
	case 71, 73, 75, 77:
		return ConditionSnow
	case 80, 81, 82:
		return ConditionShowers
	case 85, 86:
		return ConditionSnowShowers
	case 95, 96, 99:
		return ConditionThunderstorm
	}
	return ConditionCloudy
}

// response is the subset of the Open-Meteo forecast response we use.
type response struct {
	Timezone         string `json:"timezone"`
	UTCOffsetSeconds int    `json:"utc_offset_seconds"`
	Current          struct {
		Temp     float64 `json:"temperature_2m"`
		Feels    float64 `json:"apparent_temperature"`
		Humidity float64 `json:"relative_humidity_2m"`
		Precip   float64 `json:"precipitation"`
		Code     int     `json:"weather_code"`
		Wind     float64 `json:"wind_speed_10m"`
		WindDir  float64 `json:"wind_direction_10m"`
		IsDay    int     `json:"is_day"`
	} `json:"current"`
	Hourly struct {
		Time       []int64    `json:"time"`
		Temp       []*float64 `json:"temperature_2m"`
		PrecipProb []*float64 `json:"precipitation_probability"`
		Code       []*float64 `json:"weather_code"`
	} `json:"hourly"`
	Daily struct {
		Time       []int64    `json:"time"`
		Code       []*float64 `json:"weather_code"`
		Max        []*float64 `json:"temperature_2m_max"`
		Min        []*float64 `json:"temperature_2m_min"`
		PrecipProb []*float64 `json:"precipitation_probability_max"`
		Sunrise    []int64    `json:"sunrise"`
		Sunset     []int64    `json:"sunset"`
	} `json:"daily"`
}

func (r response) toData(name string, now time.Time) (Data, error) {
	loc, err := time.LoadLocation(r.Timezone)
	if err != nil {
		loc = time.FixedZone("", r.UTCOffsetSeconds)
	}
	d := Data{
		Location: name,
		Timezone: loc.String(),
		Current: Current{
			TempC:      r.Current.Temp,
			FeelsLikeC: r.Current.Feels,
			Humidity:   int(r.Current.Humidity + 0.5),
			PrecipMM:   r.Current.Precip,
			WindKmh:    r.Current.Wind,
			WindDir:    int(r.Current.WindDir + 0.5),
			Condition:  conditionFromWMO(r.Current.Code),
			IsDay:      r.Current.IsDay == 1,
		},
	}

	h := r.Hourly
	if len(h.Time) == 0 || len(h.Temp) != len(h.Time) {
		return Data{}, errors.New("weather: unexpected hourly data from upstream")
	}
	// Start with the hour that contains "now".
	start := 0
	for start < len(h.Time) && h.Time[start] < now.Unix()-3599 {
		start++
	}
	for i := start; i < len(h.Time) && len(d.Hourly) < hourlyCount; i++ {
		if h.Temp[i] == nil {
			continue
		}
		d.Hourly = append(d.Hourly, Hour{
			T:          h.Time[i],
			TempC:      *h.Temp[i],
			PrecipProb: pick(h.PrecipProb, i),
			Condition:  conditionFromWMO(pick(h.Code, i)),
		})
	}

	dl := r.Daily
	for i, t := range dl.Time {
		if i >= dailyCount || i >= len(dl.Max) || i >= len(dl.Min) || dl.Max[i] == nil || dl.Min[i] == nil {
			continue
		}
		day := Day{
			Date:       time.Unix(t, 0).In(loc).Format("2006-01-02"),
			MaxC:       *dl.Max[i],
			MinC:       *dl.Min[i],
			PrecipProb: pick(dl.PrecipProb, i),
			Condition:  conditionFromWMO(pick(dl.Code, i)),
		}
		if i < len(dl.Sunrise) {
			day.Sunrise = dl.Sunrise[i]
		}
		if i < len(dl.Sunset) {
			day.Sunset = dl.Sunset[i]
		}
		d.Daily = append(d.Daily, day)
	}
	if len(d.Hourly) == 0 {
		return Data{}, fmt.Errorf("weather: no hourly data after %s", now.Format(time.RFC3339))
	}
	return d, nil
}

// pick returns the rounded value at i, or 0 if missing (upstream uses null).
func pick(s []*float64, i int) int {
	if i < len(s) && s[i] != nil {
		return int(*s[i] + 0.5)
	}
	return 0
}
