package types

type VSTPCIFMsgV1 struct {
	SchemaLocation string       `json:"schemaLocation"`
	Classification string       `json:"classification"`
	Timestamp      string       `json:"timestamp"`
	Owner          string       `json:"owner"`
	OriginMsgId    string       `json:"originMsgId"`
	Sender         Sender       `json:"Sender"`
	Schedule       VSTPSchedule `json:"schedule"`
}

type VSTPSchedule struct {
	ScheduleId          string                `json:"schedule_id"`
	TransactionType     string                `json:"transaction_type"`
	ScheduleStartDate   string                `json:"schedule_start_date"`
	ScheduleEndDate     string                `json:"schedule_end_date"`
	ScheduleDaysRuns    string                `json:"schedule_days_runs"`
	ApplicableTimetable string                `json:"applicable_timetable"`
	BankHolidayRunning  string                `json:"CIF_bank_holiday_running"`
	TrainUID            string                `json:"CIF_train_uid"`
	TrainStatus         string                `json:"train_status"`
	StpIndicator        string                `json:"CIF_stp_indicator"`
	ScheduleSegment     []VSTPScheduleSegment `json:"schedule_segment"`
}

type VSTPScheduleSegment struct {
	SignallingId             string                 `json:"signalling_id"`
	UicCode                  string                 `json:"uic_code"`
	AtocCode                 string                 `json:"atoc_code"`
	TrainCategory            string                 `json:"CIF_train_category"`
	Headcode                 string                 `json:"CIF_headcode"`
	CourseIndicator          string                 `json:"CIF_course_indicator"`
	TrainServiceCode         string                 `json:"CIF_train_service_code"`
	BusinessSector           string                 `json:"CIF_business_sector"`
	PowerType                string                 `json:"CIF_power_type"`
	TimingLoad               string                 `json:"CIF_timing_load"`
	Speed                    string                 `json:"CIF_speed"`
	OperatingCharacteristics string                 `json:"CIF_operating_characteristics"`
	TrainClass               string                 `json:"CIF_train_class"`
	Sleepers                 string                 `json:"CIF_sleepers"`
	Reservations             string                 `json:"CIF_reservations"`
	ConnectionIndicator      string                 `json:"CIF_connection_indicator"`
	CateringCode             string                 `json:"CIF_catering_code"`
	ServiceBranding          string                 `json:"CIF_service_branding"`
	TractionClass            string                 `json:"CIF_traction_class"`
	ScheduleLocation         []VSTPScheduleLocation `json:"schedule_location"`
}

type VSTPScheduleLocation struct {
	ScheduledArrivalTime   string       `json:"scheduled_arrival_time"`
	ScheduledDepartureTime string       `json:"scheduled_departure_time"`
	ScheduledPassTime      string       `json:"scheduled_pass_time"`
	PublicArrivalTime      string       `json:"public_arrival_time"`
	PublicDepartureTime    string       `json:"public_departure_time"`
	Platform               string       `json:"CIF_platform"`
	Line                   string       `json:"CIF_line"`
	Path                   string       `json:"CIF_path"`
	Activity               string       `json:"CIF_activity"`
	EngineeringAllowance   string       `json:"CIF_engineering_allowance"`
	PathingAllowance       string       `json:"CIF_pathing_allowance"`
	PerformanceAllowance   string       `json:"CIF_performance_allowance"`
	Location               VSTPLocation `json:"location"`
}

type VSTPLocation struct {
	Tiploc VSTPTiploc `json:"tiploc"`
}

type VSTPTiploc struct {
	TiplocId string `json:"tiploc_id"`
}

type VSTPMessage struct {
	VSTPCIFMsgV1 VSTPCIFMsgV1 `json:"VSTPCIFMsgV1"`
}
