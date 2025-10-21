package types

type Stop struct {
	Stanox     string `json:"stanox"`
	PlannedArr string `json:"planned_arr,omitempty"`
	PlannedDep string `json:"planned_dep,omitempty"`
	ActualArr  string `json:"actual_arr,omitempty"`
	ActualDep  string `json:"actual_dep,omitempty"`
}

type TrainJourney struct {
	UID     string `json:"uid"`
	RunDate string `json:"run_date"`
	Stops   []Stop `json:"stops"`
}
