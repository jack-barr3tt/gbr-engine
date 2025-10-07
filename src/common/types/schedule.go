package types

type JsonTimetableV1 struct {
	Classification string   `json:"classification"`
	Timestamp      int64    `json:"timestamp"`
	Owner          string   `json:"owner"`
	Sender         Sender   `json:"Sender"`
	Metadata       Metadata `json:"Metadata"`
}

type Sender struct {
	Organisation string `json:"organisation"`
	Application  string `json:"application"`
	Component    string `json:"component"`
}

type Metadata struct {
	Type     string `json:"type"`
	Sequence int    `json:"sequence"`
}

type TiplocV1 struct {
	TransactionType string  `json:"transaction_type"`
	TiplocCode      string  `json:"tiploc_code"`
	Nalco           string  `json:"nalco"`
	Stanox          *string `json:"stanox"`
	CrsCode         *string `json:"crs_code"`
	Description     *string `json:"description"`
	TpsDescription  string  `json:"tps_description"`
}

type JsonAssociationV1 struct {
	TransactionType     string  `json:"transaction_type"`
	MainTrainUID        string  `json:"main_train_uid"`
	AssocTrainUID       string  `json:"assoc_train_uid"`
	AssocStartDate      string  `json:"assoc_start_date"`
	AssocEndDate        string  `json:"assoc_end_date"`
	AssocDays           string  `json:"assoc_days"`
	Category            string  `json:"category"`
	DateIndicator       string  `json:"date_indicator"`
	Location            string  `json:"location"`
	BaseLocationSuffix  *string `json:"base_location_suffix"`
	AssocLocationSuffix *string `json:"assoc_location_suffix"`
	DiagramType         string  `json:"diagram_type"`
	StpIndicator        string  `json:"CIF_stp_indicator"`
}

type JsonScheduleV1 struct {
	BankHolidayRunning  *string             `json:"CIF_bank_holiday_running"`
	StpIndicator        string              `json:"CIF_stp_indicator"`
	TrainUID            string              `json:"CIF_train_uid"`
	ApplicableTimetable *string             `json:"applicable_timetable"`
	AtocCode            *string             `json:"atoc_code"`
	NewScheduleSegment  *NewScheduleSegment `json:"new_schedule_segment"`
	ScheduleDaysRuns    string              `json:"schedule_days_runs"`
	ScheduleEndDate     string              `json:"schedule_end_date"`
	ScheduleSegment     ScheduleSegment     `json:"schedule_segment"`
	ScheduleStartDate   string              `json:"schedule_start_date"`
	TrainStatus         string              `json:"train_status"`
	TransactionType     string              `json:"transaction_type"`
}

type NewScheduleSegment struct {
	TractionClass string `json:"traction_class"`
	UICCode       string `json:"uic_code"`
}

type ScheduleSegment struct {
	SignallingID             string             `json:"signalling_id"`
	TrainCategory            string             `json:"CIF_train_category"`
	Headcode                 string             `json:"CIF_headcode"`
	CourseIndicator          int                `json:"CIF_course_indicator"`
	TrainServiceCode         string             `json:"CIF_train_service_code"`
	BusinessSector           string             `json:"CIF_business_sector"`
	PowerType                *string            `json:"CIF_power_type"`
	TimingLoad               *string            `json:"CIF_timing_load"`
	Speed                    *string            `json:"CIF_speed"`
	OperatingCharacteristics *string            `json:"CIF_operating_characteristics"`
	TrainClass               *string            `json:"CIF_train_class"`
	Sleepers                 *string            `json:"CIF_sleepers"`
	Reservations             *string            `json:"CIF_reservations"`
	ConnectionIndicator      *string            `json:"CIF_connection_indicator"`
	CateringCode             *string            `json:"CIF_catering_code"`
	ServiceBranding          string             `json:"CIF_service_branding"`
	ScheduleLocation         []ScheduleLocation `json:"schedule_location"`
}

type ScheduleLocation struct {
	LocationType         string  `json:"location_type"`
	RecordIdentity       string  `json:"record_identity"`
	TiplocCode           string  `json:"tiploc_code"`
	TiplocInstance       *string `json:"tiploc_instance"`
	Departure            *string `json:"departure"`
	PublicDeparture      *string `json:"public_departure"`
	Platform             *string `json:"platform"`
	Line                 *string `json:"line"`
	EngineeringAllowance *string `json:"engineering_allowance"`
	PathingAllowance     *string `json:"pathing_allowance"`
	PerformanceAllowance *string `json:"performance_allowance"`
	Arrival              *string `json:"arrival"`
	PublicArrival        *string `json:"public_arrival"`
	Pass                 *string `json:"pass"`
	Path                 *string `json:"path"`
}

type EOFMessage struct {
	EOF bool `json:"EOF"`
}

type TimetableEntry struct {
	JsonTimetableV1   *JsonTimetableV1   `json:"JsonTimetableV1"`
	TiplocV1          *TiplocV1          `json:"TiplocV1"`
	JsonAssociationV1 *JsonAssociationV1 `json:"JsonAssociationV1"`
	JsonScheduleV1    *JsonScheduleV1    `json:"JsonScheduleV1"`
	EOF               *EOFMessage        `json:"EOFMessage"`
}
