package oura

const baseURL = "https://api.ouraring.com"

// EndpointSpec describes a single Oura API v2 endpoint.
type EndpointSpec struct {
	Name     string // e.g. "daily_sleep"
	Path     string // e.g. "/v2/usercollection/daily_sleep"
	HasDates bool   // supports start_date/end_date query params
	IsList   bool   // returns paginated list vs single object
	IDField  string // JSON field to use as oura_id ("id", "" for none)
	DayField string // JSON field to use as day ("day", "timestamp", "")
}

// Registry contains all Oura API v2 endpoints.
var Registry = []EndpointSpec{
	{Name: "daily_sleep", Path: "/v2/usercollection/daily_sleep", HasDates: true, IsList: true, IDField: "id", DayField: "day"},
	{Name: "sleep", Path: "/v2/usercollection/sleep", HasDates: true, IsList: true, IDField: "id", DayField: "day"},
	{Name: "sleep_time", Path: "/v2/usercollection/sleep_time", HasDates: true, IsList: true, IDField: "id", DayField: "day"},
	{Name: "daily_activity", Path: "/v2/usercollection/daily_activity", HasDates: true, IsList: true, IDField: "id", DayField: "day"},
	{Name: "daily_readiness", Path: "/v2/usercollection/daily_readiness", HasDates: true, IsList: true, IDField: "id", DayField: "day"},
	{Name: "heartrate", Path: "/v2/usercollection/heartrate", HasDates: true, IsList: true, IDField: "", DayField: "timestamp"},
	{Name: "daily_resilience", Path: "/v2/usercollection/daily_resilience", HasDates: true, IsList: true, IDField: "id", DayField: "day"},
	{Name: "daily_stress", Path: "/v2/usercollection/daily_stress", HasDates: true, IsList: true, IDField: "id", DayField: "day"},
	{Name: "daily_spo2", Path: "/v2/usercollection/daily_spo2", HasDates: true, IsList: true, IDField: "id", DayField: "day"},
	{Name: "daily_cardiovascular_age", Path: "/v2/usercollection/daily_cardiovascular_age", HasDates: true, IsList: true, IDField: "id", DayField: "day"},
	{Name: "vo2_max", Path: "/v2/usercollection/vo2_max", HasDates: true, IsList: true, IDField: "id", DayField: "day"},
	{Name: "workout", Path: "/v2/usercollection/workout", HasDates: true, IsList: true, IDField: "id", DayField: "day"},
	{Name: "session", Path: "/v2/usercollection/session", HasDates: true, IsList: true, IDField: "id", DayField: "day"},
	{Name: "tag", Path: "/v2/usercollection/tag", HasDates: true, IsList: true, IDField: "id", DayField: "day"},
	{Name: "enhanced_tag", Path: "/v2/usercollection/enhanced_tag", HasDates: true, IsList: true, IDField: "id", DayField: "day"},
	{Name: "ring_configuration", Path: "/v2/usercollection/ring_configuration", HasDates: false, IsList: true, IDField: "id", DayField: ""},
	{Name: "rest_mode_period", Path: "/v2/usercollection/rest_mode_period", HasDates: true, IsList: true, IDField: "id", DayField: "start_day"},
	{Name: "personal_info", Path: "/v2/usercollection/personal_info", HasDates: false, IsList: false, IDField: "", DayField: ""},
}

// RegistryMap provides lookup by endpoint name.
var RegistryMap = func() map[string]EndpointSpec {
	m := make(map[string]EndpointSpec, len(Registry))
	for _, spec := range Registry {
		m[spec.Name] = spec
	}
	return m
}()

// EndpointNames returns all endpoint names.
func EndpointNames() []string {
	names := make([]string, len(Registry))
	for i, spec := range Registry {
		names[i] = spec.Name
	}
	return names
}
