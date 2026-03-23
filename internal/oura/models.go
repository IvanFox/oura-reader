package oura

// Models are defined progressively. Storage uses json.RawMessage.
// These typed structs are available for future use when richer API responses are needed.

type DailySleep struct {
	ID           string              `json:"id"`
	Day          string              `json:"day"`
	Score        *int                `json:"score"`
	Timestamp    string              `json:"timestamp"`
	Contributors *SleepContributors  `json:"contributors"`
}

type SleepContributors struct {
	DeepSleep    *int `json:"deep_sleep"`
	Efficiency   *int `json:"efficiency"`
	Latency      *int `json:"latency"`
	REMSleep     *int `json:"rem_sleep"`
	Restfulness  *int `json:"restfulness"`
	Timing       *int `json:"timing"`
	TotalSleep   *int `json:"total_sleep"`
}

type DailyReadiness struct {
	ID                       string                  `json:"id"`
	Day                      string                  `json:"day"`
	Score                    *int                    `json:"score"`
	Timestamp                string                  `json:"timestamp"`
	TemperatureDeviation     *float64                `json:"temperature_deviation"`
	TemperatureTrendDeviation *float64               `json:"temperature_trend_deviation"`
	Contributors             *ReadinessContributors  `json:"contributors"`
}

type ReadinessContributors struct {
	BodyTemperature      *int `json:"body_temperature"`
	ActivityBalance      *int `json:"activity_balance"`
	HRVBalance           *int `json:"hrv_balance"`
	RecoveryIndex        *int `json:"recovery_index"`
	RestingHeartRate     *int `json:"resting_heart_rate"`
	SleepBalance         *int `json:"sleep_balance"`
	PreviousNight        *int `json:"previous_night"`
	PreviousDayActivity  *int `json:"previous_day_activity"`
}

type DailyActivity struct {
	ID           string                 `json:"id"`
	Day          string                 `json:"day"`
	Score        *int                   `json:"score"`
	Timestamp    string                 `json:"timestamp"`
	Contributors *ActivityContributors  `json:"contributors"`
}

type ActivityContributors struct {
	MeetDailyTargets   *int `json:"meet_daily_targets"`
	MoveEveryHour      *int `json:"move_every_hour"`
	RecoveryTime       *int `json:"recovery_time"`
	StayActive         *int `json:"stay_active"`
	TrainingFrequency  *int `json:"training_frequency"`
	TrainingVolume     *int `json:"training_volume"`
}

type HeartRate struct {
	BPM       int    `json:"bpm"`
	Source    string `json:"source"`
	Timestamp string `json:"timestamp"`
}
